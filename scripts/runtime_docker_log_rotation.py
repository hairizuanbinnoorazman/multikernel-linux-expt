"""Metadata-only acceptance for an owned disposable Docker local-log sample."""
import os
from pathlib import Path
import re
import stat
from runtime_trial_output import require_logging

MAX_FILE = 1_000_000
RECORD_SLACK = 65536


def qualify_rotation(item, data_root):
    """Require rotation and a conservative observed bound; never read logs."""
    require_logging(item['HostConfig'].get('LogConfig'))
    if item['State']['Status'] != 'exited' or type(item['State']['ExitCode']) is not int or item['State']['ExitCode'] != 0:
        raise ValueError('logging workload did not complete')
    identifier = item['Id']
    if type(identifier) is not str or not re.fullmatch('[a-f0-9]{64}', identifier):
        raise ValueError('invalid logging container identity')
    directory = Path(data_root)/'containers'/identifier/'local-logs'
    # Docker deliberately leaves this API field empty for the local driver.
    # Derive the lab-only internal path from the pinned data root and ID;
    # never accept a caller-provided path to a log file.
    if item.get('LogPath') != '':
        raise ValueError('local logging API path is unexpected')
    # Open every component relative to held directories; aliases cannot turn
    # an observation into a traversal through primary daemon state.
    descriptor = os.open(data_root, os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC)
    try:
        for part in ['containers', identifier, 'local-logs']:
            next_fd = os.open(part, os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            os.close(descriptor)
            descriptor = next_fd
        if set(os.listdir(descriptor)) != {'container.log', 'container.log.1'}:
            raise ValueError('local log rotation sample differs')
        sizes = []
        for name in ['container.log', 'container.log.1']:
            fd = os.open(name, os.O_PATH | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=descriptor)
            try:
                info = os.fstat(fd)
                if (not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or
                        info.st_nlink != 1 or info.st_mode & 0o022 or
                        not 0 < info.st_size <= MAX_FILE + RECORD_SLACK):
                    raise ValueError('unsafe or oversized local log sample')
                sizes.append(info.st_size)
            finally:
                os.close(fd)
        if sizes[1] < MAX_FILE:
            raise ValueError('rotated sample did not reach configured threshold')
        return dict(rotated_file_count=2, observed_bytes=sum(sizes),
                    conservative_sample_bound=2*(MAX_FILE+RECORD_SLACK),
                    log_contents_read=False)
    finally:
        os.close(descriptor)
