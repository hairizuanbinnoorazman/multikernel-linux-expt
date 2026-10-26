"""Read-only minimum-capacity guard before a disposable child Docker trial.

This is a necessary floor, not a reservation or complete builder admission.
Source staging, concurrent writers and all builder checks remain authoritative.
"""
import hashlib
import os
from pathlib import Path
import time

IMAGE_BYTES = 2 << 30
RESERVE_BYTES = 1 << 30
RESERVE_INODES = 1024


def observe(path, image_count=1):
    path = Path(path)
    if not path.is_absolute() or '..' in path.parts or type(image_count) is not int or image_count not in (1, 2):
        raise ValueError('invalid private trial capacity scope')
    descriptor = os.open('/', os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        # Pin the selected filesystem without following any symlink component.
        for component in path.parts[1:]:
            next_descriptor = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = next_descriptor
        identity = os.fstat(descriptor)
        capacity = os.fstatvfs(descriptor)
    finally:
        os.close(descriptor)
    free_bytes = capacity.f_bavail * capacity.f_frsize
    free_inodes = capacity.f_favail
    required_bytes = image_count * IMAGE_BYTES + RESERVE_BYTES
    required_inodes = image_count * 4 + RESERVE_INODES
    reasons = []
    if free_bytes < required_bytes:
        reasons.append('MINIMUM_FREE_BYTES')
    if free_inodes < required_inodes:
        reasons.append('MINIMUM_FREE_INODES')
    return dict(schema_version=1, scope='child-docker-trial-minimum-capacity',
                observed_monotonic_ns=time.monotonic_ns(),
                path_sha256=hashlib.sha256(os.fsencode(path)).hexdigest(),
                directory_identity=dict(device=identity.st_dev, inode=identity.st_ino),
                image_count=image_count, image_bytes=IMAGE_BYTES,
                reserve_bytes=RESERVE_BYTES, reserve_inodes=RESERVE_INODES,
                free_bytes=free_bytes, free_inodes=free_inodes,
                minimum_required_bytes=required_bytes, minimum_required_inodes=required_inodes,
                minimum_capacity_available=not reasons, refusal_codes=reasons,
                capacity_reserved=False, source_staging_accounted=False,
                full_builder_admission_claimed=False, cleanup_qualified=False)


def require(report):
    if not report['minimum_capacity_available']:
        raise RuntimeError('child Docker trial capacity floor refused: ' + ','.join(report['refusal_codes']))
