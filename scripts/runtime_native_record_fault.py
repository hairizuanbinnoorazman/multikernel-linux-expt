"""Private lab fault: temporarily hide one exact live export process record.

Not a repair tool. A replaced/modified held record or occupied restore target
refuses without overwriting either file. A killed injector can leave the held
record for diagnosis; restart/recovery must not run while the record is hidden.
"""
import ctypes
import hashlib
import json
import os
import pathlib
import re
import signal
import stat
import uuid


def _unique(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('ambiguous native process record')
        result[key] = value
    return result


def _identity(s):
    return (s.st_dev, s.st_ino, s.st_uid, s.st_gid, s.st_mode, s.st_nlink,
            s.st_size, s.st_mtime_ns, s.st_ctime_ns)


def _open_directory(path):
    path = pathlib.Path(path)
    if not path.is_absolute() or str(path) != os.path.normpath(path) or '..' in path.parts:
        raise ValueError('private directory must be canonical and absolute')
    fd = os.open('/', os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        for part in path.parts[1:]:
            next_fd = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=fd)
            os.close(fd)
            fd = next_fd
        s = os.fstat(fd)
        if s.st_uid != os.geteuid() or s.st_mode & 0o077:
            raise ValueError('native runtime directory is not caller-private')
        return fd
    except BaseException:
        os.close(fd)
        raise


def _read(fd, name):
    file = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC, dir_fd=fd)
    try:
        s = os.fstat(file)
        if (not stat.S_ISREG(s.st_mode) or s.st_uid != os.geteuid() or
                stat.S_IMODE(s.st_mode) != 0o600 or s.st_nlink != 1 or not 0 < s.st_size <= 4096):
            raise ValueError('native process record is not private and bounded')
        data = os.read(file, 4097)
        named = os.stat(name, dir_fd=fd, follow_symlinks=False)
        if len(data) != s.st_size or _identity(s) != _identity(os.fstat(file)) or _identity(named) != _identity(s):
            raise ValueError('native process record changed during read')
        return data, s
    finally:
        os.close(file)


def _rename(fd, source, target):
    libc = ctypes.CDLL(None, use_errno=True)
    # RENAME_NOREPLACE is required for both hide and restore. Never fall back to
    # os.rename, whose replace semantics could destroy conflicting evidence.
    call = libc.renameat2
    call.argtypes = [ctypes.c_int, ctypes.c_char_p, ctypes.c_int, ctypes.c_char_p, ctypes.c_uint]
    call.restype = ctypes.c_int
    if call(fd, os.fsencode(source), fd, os.fsencode(target), 1):
        raise OSError(ctypes.get_errno(), 'exact native record rename refused')


class HiddenNativeRecord:
    def __init__(self, directory, export, binary, placeholder=None):
        if placeholder not in [None, 'fifo']:
            raise ValueError('unsupported native record placeholder')
        self.placeholder = placeholder
        self.fifo_identity = None
        self.directory = pathlib.Path(directory)
        self.export = export
        self.binary = str(binary)
        self.fd = self.pidfd = None
        self.hidden = False

    def _live(self):
        signal.pidfd_send_signal(self.pidfd, 0)
        proc = pathlib.Path('/proc') / str(self.record['pid'])
        data = (proc / 'stat').read_bytes()
        if len(data) > 4096 or int(data[data.rindex(b')') + 2:].split()[19]) != self.record['start_time']:
            raise ValueError('native process incarnation changed')
        expected = '\0'.join([self.binary, 'server', '/proc/self/fd/3', str(self.record['port']),
                              self.record['image_id'], self.record['export_generation'],
                              str(self.record['peer_cid']), '']).encode()
        with (proc / 'cmdline').open('rb') as file:
            command = file.read(4097)
        if command != expected:
            raise ValueError('native process command changed')
        for name, dev, ino in [('exe', 'binary_device', 'binary_inode'), ('fd/3', 'image_device', 'image_inode')]:
            s = (proc / name).stat()
            if (s.st_dev, s.st_ino) != (self.record[dev], self.record[ino]):
                raise ValueError('native process descriptor identity changed')
        signal.pidfd_send_signal(self.pidfd, 0)

    def _same_directory(self):
        current = _open_directory(self.directory)
        try:
            if _identity(os.fstat(current))[:6] != _identity(os.fstat(self.fd))[:6]:
                raise ValueError('native runtime directory was replaced')
        finally:
            os.close(current)

    def check_hidden(self):
        self._same_directory()
        data, s = _read(self.fd, self.held)
        if data != self.data or _identity(s) != self.held_identity:
            raise ValueError('held native record changed')
        if self.fifo_identity is not None:
            named = os.stat(self.name, dir_fd=self.fd, follow_symlinks=False)
            if _identity(named) != self.fifo_identity:
                raise ValueError('native FIFO placeholder changed')
        else:
            try:
                os.stat(self.name, dir_fd=self.fd, follow_symlinks=False)
            except FileNotFoundError:
                pass
            else:
                raise ValueError('native process record was recreated')
        self._live()

    def __enter__(self):
        try:
            e = self.export
            if (not isinstance(e['sandbox_id'], str) or not re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}', e['sandbox_id']) or
                    not re.fullmatch('[a-f0-9]{32}', e['sandbox_generation']) or e['state'] != 'ACTIVE'):
                raise ValueError('expected one exact active private export')
            self.name = e['sandbox_id'] + '-' + e['sandbox_generation'][:12] + '.json'
            self.held = '.g8-diagnostic-hold-' + uuid.uuid4().hex
            self.fd = _open_directory(self.directory)
            self.data, original = _read(self.fd, self.name)
            r = self.record = json.loads(self.data, object_pairs_hook=_unique)
            expected = dict(version=4, path=e['path'], port=e['port'], image_id=e['image_id'],
                            export_generation=e['export_generation'], peer_cid=e['peer_cid'],
                            image_device=e['image_identity']['device'], image_inode=e['image_identity']['inode'])
            if (not isinstance(r, dict) or set(r) != set(expected) | {'pid', 'start_time', 'binary_device', 'binary_inode'} or
                    any(r.get(k) != v or type(r.get(k)) is not type(v) for k, v in expected.items()) or
                    any(type(r[k]) is not int or r[k] <= 0 for k in ['pid', 'start_time', 'binary_device', 'binary_inode']) or r['pid'] <= 1):
                raise ValueError('native record differs from the exact export')
            self.pidfd = os.pidfd_open(r['pid'])
            self._live()
            self._same_directory()
            latest, s = _read(self.fd, self.name)
            if latest != self.data or _identity(s) != _identity(original):
                raise ValueError('native record changed before fault')
            _rename(self.fd, self.name, self.held)
            self.hidden = True
            data, held = _read(self.fd, self.held)
            if data != self.data or (held.st_dev, held.st_ino) != (original.st_dev, original.st_ino):
                raise ValueError('renamed native record differs')
            self.held_identity = _identity(held)
            if self.placeholder == 'fifo':
                os.mkfifo(self.name, 0o600, dir_fd=self.fd)
                named = os.stat(self.name, dir_fd=self.fd, follow_symlinks=False)
                self.fifo_identity = _identity(named)
                if not stat.S_ISFIFO(named.st_mode) or named.st_nlink != 1 or stat.S_IMODE(named.st_mode) != 0o600:
                    raise ValueError('native FIFO placeholder identity is unsafe')
            os.fsync(self.fd)
            self.check_hidden()
            return self
        except BaseException:
            self.__exit__(None, None, None)
            raise

    def __exit__(self, *unused):
        try:
            if self.hidden:
                # Restore even when the client/doctor fails. A dead server does
                # not authorize destroying the record; only exact held bytes
                # and inode authorize restoring the original pathname.
                if not hasattr(self, 'held_identity'):
                    raise ValueError('unverified held record preserved for diagnosis')
                data, held = _read(self.fd, self.held)
                if data != self.data or _identity(held) != self.held_identity:
                    raise ValueError('refusing restoration of modified held record')
                self._same_directory()
                if self.fifo_identity is not None:
                    named = os.stat(self.name, dir_fd=self.fd, follow_symlinks=False)
                    if _identity(named) != self.fifo_identity:
                        raise ValueError('refusing removal of changed FIFO placeholder')
                    quarantine = '.g8-fifo-remove-' + uuid.uuid4().hex
                    _rename(self.fd, self.name, quarantine)
                    moved = os.stat(quarantine, dir_fd=self.fd, follow_symlinks=False)
                    # Our rename changes ctime. All other identity fields must
                    # still describe the exact FIFO; preserve conflicts.
                    if _identity(moved)[:-1] != self.fifo_identity[:-1]:
                        _rename(self.fd, quarantine, self.name)
                        raise ValueError('FIFO identity changed during removal')
                    os.unlink(quarantine, dir_fd=self.fd)
                    self.fifo_identity = None
                _rename(self.fd, self.held, self.name)
                self.hidden = False
                os.fsync(self.fd)
                data, restored = _read(self.fd, self.name)
                if data != self.data or (restored.st_dev, restored.st_ino) != (held.st_dev, held.st_ino):
                    raise ValueError('native record restoration differs')
        finally:
            for fd in [self.pidfd, self.fd]:
                if fd is not None:
                    os.close(fd)
            self.pidfd = self.fd = None

    def proof(self):
        return dict(placeholder='fifo' if self.placeholder else 'missing',
                    placeholder_identity=list(self.fifo_identity) if self.fifo_identity else None,
                    no_fifo_writer_opened=True, record_sha256=hashlib.sha256(self.data).hexdigest(),
                    record_device=self.held_identity[0], record_inode=self.held_identity[1],
                    process_live=True, exact_process_incarnation=True,
                    launched_executable_and_backing_fd_match=True)
