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


def filtered_events(data):
    result = []
    for line in data.decode(errors='replace').splitlines():
        if re.search(r'nbd|mkvsock|forward_stream|ext4|out of memory|oom|killed process|I/O error', line, re.I):
            line = re.sub(r'\b[0-9a-fA-F]{64}\b', '[REDACTED_HEX64]', line)
            line = re.sub(r'(?i)\b(token|password|secret|authorization)\s*[=:].*', r'\1=[REDACTED]', line)
            result.append(line[:1024])
    return ('\n'.join(result) + ('\n' if result else '')).encode()[-65536:]


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
        self.attached = threading.Event()
        self.record = {'instance': instance, 'attached': False, 'bytes': 0}
        self.thread = threading.Thread(target=self._capture, daemon=True)

    def start(self):
        self.thread.start()

    def _capture(self):
        fd = None
        data = b''
        try:
            instance = pathlib.Path('/sys/fs/multikernel/instances') / self.instance
            deadline = time.monotonic() + 300
            while not self.stop.is_set():
                try:
                    if (instance / 'status').read_text().strip() == 'active':
                        break
                except FileNotFoundError:
                    pass
                if time.monotonic() >= deadline:
                    self.record['reason'] = 'attachment-deadline'
                    return
                self.stop.wait(.05)
            if self.stop.is_set():
                self.record['reason'] = 'stopped-before-attachment'
                return
            identity = instance.stat()
            cid_text = (instance / 'id').read_text().strip()
            if not re.fullmatch(r'[1-9][0-9]{0,2}', cid_text) or not 3 <= int(cid_text) <= 511:
                raise ValueError('invalid console instance ID')
            fd = os.open('/dev/mktty', os.O_RDWR | os.O_NONBLOCK | os.O_CLOEXEC | os.O_NOFOLLOW)
            device = os.fstat(fd)
            if not stat.S_ISCHR(device.st_mode) or device.st_uid != 0 or os.major(device.st_rdev) != 10:
                raise ValueError('unexpected protected mktty device')
            selection = (cid_text + '\n').encode()
            if os.write(fd, selection) != len(selection):
                raise OSError(errno.EIO, 'short console selection')
            after = instance.stat()
            if (identity.st_dev, identity.st_ino) != (after.st_dev, after.st_ino):
                raise ValueError('console instance changed during attachment')
            self.record.update(attached=True, cid=int(cid_text),
                               instance_identity={'device': identity.st_dev, 'inode': identity.st_ino})
            self.attached.set()
            data, self.record['reason'] = read_bounded(fd, self.stop, empty_is_idle=True)
        except (OSError, ValueError) as error:
            self.record.update(reason='capture-error', error_type=type(error).__name__)
            if isinstance(error, OSError):
                self.record['errno'] = error.errno
        finally:
            if fd is not None:
                os.close(fd)
            self.record.update(bytes=len(data), sha256=hashlib.sha256(data).hexdigest())
            # Raw output remains private and is excluded from evidence exports.
            for name, payload in [('console-private.log', data),
                                  ('console-events.log', filtered_events(data))]:
                handle = os.open(self.output / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(handle, 'wb') as stream:
                    stream.write(payload)
            (self.output / 'console-capture.json').write_text(json.dumps(self.record, indent=2) + '\n')

    def finish(self):
        self.stop.set()
        self.thread.join(timeout=2)
        if self.thread.is_alive():
            raise RuntimeError('console observer did not stop within its bound')
