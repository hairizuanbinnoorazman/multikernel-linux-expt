"""Pinned native helper selection for disposable GCE trials, never production."""
import hashlib
import os
from pathlib import Path
import stat

PINS = {
    'qualified': 'c9981a1554230e87098472b586ab0fbe20f01c0b813fbd95df5b5f5c99d4a389',
    'server-queue-candidate': '591f79b081da9226780cffb8cf1db6e697f72313120aa1c9019218399eb4f428',
}


# Static native artifact already qualified on real CID/peer/replacement paths.
RELAY_PIN = '54bc9b1bd1c7db8cf39229bc6d268f6e2d8231d82ee3149d33293dac6386afd9'


def _pinned_file(path, expected_digest):
    path = Path(path)
    if not path.is_absolute() or '..' in path.parts:
        raise ValueError('invalid disposable trial helper path')
    for directory in [path.parent, *path.parent.parents]:
        info = directory.lstat()
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != 0 or
                (info.st_mode & 0o022 and not (directory != path.parent and info.st_mode & stat.S_ISVTX))):
            raise ValueError('unprotected disposable trial helper directory')
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(fd, 'rb') as file:
        info = os.fstat(file.fileno())
        if (not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or info.st_nlink != 1 or
                info.st_mode & 0o022 or not info.st_mode & 0o111 or not 0 < info.st_size <= 16 << 20):
            raise ValueError('unprotected disposable trial helper file')
        digest = hashlib.file_digest(file, 'sha256').hexdigest()
        if digest != expected_digest:
            raise ValueError('disposable trial helper digest differs from the selected pin')
    return info, digest


def verify(path, variant='qualified'):
    if variant not in PINS:
        raise ValueError('unknown disposable trial helper variant')
    info, digest = _pinned_file(path, PINS[variant])
    return dict(schema_version=1, helper_variant=variant, helper_sha256=digest,
                file_identity=dict(device=info.st_dev, inode=info.st_ino),
                size_bytes=info.st_size, static_amd64_pin=True,
                candidate_selected=variant != 'qualified', production_default_changed=False,
                live_health_observed=False)


def verify_relay(path):
    info, digest = _pinned_file(path, RELAY_PIN)
    return dict(schema_version=1, helper_role='agent-relay', interface='server-peer',
                helper_sha256=digest,
                file_identity=dict(device=info.st_dev, inode=info.st_ino),
                size_bytes=info.st_size, static_amd64_pin=True,
                wildcard_fallback_allowed=False,
                installed_runtime_changed=False, live_health_observed=False)
