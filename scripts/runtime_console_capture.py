"""Bounded, input-free console observation for an exact disposable instance."""
import errno
import hashlib
import json
import os
import pathlib
import re
import select
import stat
import threading
import time

LIMIT = 1024 * 1024
TEARDOWN_LIMIT = 256 * 1024


def filtered_events(data):
    result = []
    for line in data.decode(errors='replace').splitlines():
        if re.search(r'nbd|mkvsock|forward_stream|ext4|out of memory|oom|killed process|I/O error', line, re.I) or re.match(
                r'MK_STORAGE_BOOTSTRAP_(?:READY|FAIL)\b|MK_AGENT_(?:START|BOOT_CONFIG_MISSING|INSECURE_BOOT_CREDENTIAL)\b', line):
            line = re.sub(r'\b[0-9a-fA-F]{64}\b', '[REDACTED_HEX64]', line)
            line = re.sub(r'(?i)\b(token|password|secret|authorization)\s*[=:].*', r'\1=[REDACTED]', line)
            result.append(line[:1024])
    encoded = ('\n'.join(result) + ('\n' if result else '')).encode()
    return encoded[-65536:].decode(errors='ignore').encode()


def read_bounded(fd, stop, limit=LIMIT, timeout=600, empty_is_idle=False):
    data = bytearray()
    deadline = time.monotonic() + timeout
    while not stop.is_set() and time.monotonic() < deadline:
        try:
            if not select.select([fd], [], [], .05)[0]:
                continue
            part = os.read(fd, min(4096, limit + 1 - len(data)))
        except BlockingIOError:
            continue
        except OSError as error:
            return bytes(data), 'read-error-' + str(error.errno)
        if not part:
            if empty_is_idle:
                stop.wait(.05)
                continue
            return bytes(data), 'eof'
        data.extend(part)
        if len(data) > limit:
            return bytes(data[:limit]), 'output-limit'
    return bytes(data), 'stopped' if stop.is_set() else 'deadline'


class ConsoleCapture:
    def __init__(self, instance, output_directory):
        if not re.fullmatch(r'mk-[0-9a-f]{32}-[0-9a-f]{16}', instance):
            raise ValueError('expected exact Docker sandbox identity')
        self.instance = instance
        self.output = pathlib.Path(output_directory)
        self.stop = threading.Event()
        self.teardown = threading.Event()
        self.teardown_attached = threading.Event()
        self.attached = threading.Event()
        self.record = {'instance': instance, 'attached': False, 'bytes': 0}
        self.thread = threading.Thread(target=self._capture, daemon=True)

    def start(self):
        self.thread.start()

    def _connect(self, instance, expected=None):
        identity = instance.stat()
        cid_text = (instance / 'id').read_text().strip()
        if (not re.fullmatch(r'[1-9][0-9]{0,2}', cid_text) or
                not 3 <= int(cid_text) <= 511 or
                (expected is not None and expected != (identity.st_dev, identity.st_ino, int(cid_text)))):
            raise ValueError('console instance identity changed')
        fd = os.open('/dev/mktty', os.O_RDWR | os.O_NONBLOCK | os.O_CLOEXEC | os.O_NOFOLLOW)
        try:
            device = os.fstat(fd)
            if not stat.S_ISCHR(device.st_mode) or device.st_uid != 0 or os.major(device.st_rdev) != 10:
                raise ValueError('unexpected protected mktty device')
            selection = (cid_text + '\n').encode()
            # This is the only write: first-write selection, never guest input.
            if os.write(fd, selection) != len(selection):
                raise OSError(errno.EIO, 'short console selection')
            after = instance.stat()
            if (identity.st_dev, identity.st_ino) != (after.st_dev, after.st_ino):
                raise ValueError('console instance changed during attachment')
            return fd, (identity.st_dev, identity.st_ino, int(cid_text))
        except BaseException:
            os.close(fd)
            raise

    def _await_instance(self, instance):
        deadline = time.monotonic() + 300
        while not self.stop.is_set():
            try:
                if (instance / 'status').read_text().strip() == 'active':
                    return True
            except FileNotFoundError:
                pass
            if time.monotonic() >= deadline:
                self.record['reason'] = 'attachment-deadline'
                return False
            self.stop.wait(.05)
        self.record['reason'] = 'stopped-before-attachment'
        return False

    def _capture(self):
        fd = None
        segments = []
        self.record.update(coverage='phase-windows', complete_stream=False, phases=[])
        try:
            instance = pathlib.Path('/sys/fs/multikernel/instances') / self.instance
            if not self._await_instance(instance):
                return
            deadline = time.monotonic() + 600
            fd, identity = self._connect(instance)
            self.record.update(attached=True, cid=identity[2],
                               instance_identity={'device': identity[0], 'inode': identity[1]})
            self.attached.set()
            data, reason = read_bounded(fd, self.teardown, limit=LIMIT-TEARDOWN_LIMIT,
                                        timeout=max(0, deadline-time.monotonic()), empty_is_idle=True)
            segments.append(data)
            self.record['phases'].append(self._phase('startup', data, reason, LIMIT-TEARDOWN_LIMIT))
            # Close the saturated connection. Do not spend primary CPU draining
            # arbitrary workload output while waiting for the owned teardown.
            os.close(fd)
            fd = None
            while not self.teardown.is_set() and not self.stop.is_set() and time.monotonic() < deadline:
                self.teardown.wait(.05)
            if self.stop.is_set():
                self.record['reason'] = 'stopped-before-teardown'
                return
            if time.monotonic() >= deadline:
                self.record['reason'] = 'deadline-before-teardown'
                return
            # A new connection receives new output, not the old receive queue.
            # Keep the same sysfs inode and CID; refuse replacement/reuse.
            fd, _ = self._connect(instance, expected=identity)
            self.teardown_attached.set()
            data, reason = read_bounded(fd, self.stop, limit=TEARDOWN_LIMIT,
                                        timeout=max(0, deadline-time.monotonic()), empty_is_idle=True)
            segments.append(data)
            self.record['phases'].append(self._phase('teardown', data, reason, TEARDOWN_LIMIT))
            self.record['reason'] = reason
        except (OSError, ValueError) as error:
            self.record.update(reason='capture-error', error_type=type(error).__name__)
            if isinstance(error, OSError):
                self.record['errno'] = error.errno
        finally:
            if fd is not None:
                os.close(fd)
            data = b''.join(segments)
            self.record.update(bytes=len(data), sha256=hashlib.sha256(data).hexdigest(),
                               teardown_attached=self.teardown_attached.is_set(),
                               truncated=any(p['reason'] == 'output-limit' for p in self.record['phases']))
            # Filter each segment separately: a cut line in one window cannot
            # combine with the next window to fabricate a diagnostic event.
            events = b''.join(filtered_events(part[:part.rfind(b'\n')+1]) for part in segments)
            events = events[-65536:].decode(errors='ignore').encode()
            self.record.update(filtered_bytes=len(events), filtered_sha256=hashlib.sha256(events).hexdigest())
            # The raw concatenation is private and explicitly discontinuous.
            for name, payload in [('console-private.log', data), ('console-events.log', events)]:
                handle = os.open(self.output / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(handle, 'wb') as stream:
                    stream.write(payload)
            (self.output / 'console-capture.json').write_text(json.dumps(self.record, indent=2) + '\n')

    @staticmethod
    def _phase(name, data, reason, limit):
        return {'name': name, 'bytes': len(data), 'sha256': hashlib.sha256(data).hexdigest(),
                'reason': reason, 'limit_bytes': limit}

    def begin_teardown(self):
        if self.stop.is_set():
            raise RuntimeError('console observer is already stopped')
        self.teardown.set()
        if not self.teardown_attached.wait(timeout=2):
            raise RuntimeError('console teardown attachment did not complete within its bound')

    def finish(self):
        self.stop.set()
        self.teardown.set()
        self.thread.join(timeout=2)
        if self.thread.is_alive():
            raise RuntimeError('console observer did not stop within its bound')
