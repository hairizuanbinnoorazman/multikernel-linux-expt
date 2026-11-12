"""Metadata-only acceptance for an owned disposable Docker local-log sample."""
import os
from pathlib import Path
import re
import stat
from runtime_trial_output import require_logging

MAX_FILE = 1_000_000
RECORD_SLACK = 65536


def cleanup_empty_scratch(path):
    """Remove only empty directories in this invocation's exclusive scratch.

    Validate the complete set first. Never recurse or remove file contents.
    The caller must have stopped every writer and own the fresh directory.
    """
    root=os.open(path,os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC)
    held=[]
    try:
        parent=os.fstat(root)
        if parent.st_uid!=os.geteuid() or parent.st_mode&0o077:
            raise ValueError('unprotected logging scratch')
        names=os.listdir(root)
        for name in names:
            fd=os.open(name,os.O_DIRECTORY|os.O_NOFOLLOW|os.O_CLOEXEC,dir_fd=root);held.append((name,fd))
            info=os.fstat(fd)
            if info.st_uid!=os.geteuid() or info.st_mode&0o077 or os.listdir(fd):
                raise ValueError('logging scratch is not an owned empty directory')
        for name,fd in held:
            before=os.fstat(fd);named=os.stat(name,dir_fd=root,follow_symlinks=False)
            if (before.st_dev,before.st_ino)!=(named.st_dev,named.st_ino) or os.listdir(fd):
                raise ValueError('logging scratch changed')
            os.rmdir(name,dir_fd=root)
        if os.listdir(root):raise ValueError('logging scratch remains')
        return dict(empty_directories_removed=len(held),file_contents_removed=False)
    finally:
        for _,fd in held:os.close(fd)
        os.close(root)


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
