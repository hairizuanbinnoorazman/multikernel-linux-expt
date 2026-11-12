"""Fixed host-output policy for disposable Docker trial attach clients only."""
import os
import stat

ATTACH_LIMIT = 131072


def attach_command(argv):
    if not isinstance(argv, (list, tuple)) or not argv or any(type(arg) is not str or '\x00' in arg for arg in argv) or not argv[0]:
        raise ValueError('invalid attached trial command')
    # Apply the limit in the exec child, never via threaded Python preexec_fn
    # or by changing the controller/runtime/storage helper's file-size limits.
    return ['/usr/bin/prlimit', '--fsize='+str(ATTACH_LIMIT)+':'+str(ATTACH_LIMIT), '--core=0:0', '--', *argv]


def read_output(path):
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK | os.O_CLOEXEC)
    with os.fdopen(fd, 'rb') as stream:
        if not stat.S_ISREG(os.fstat(stream.fileno()).st_mode):
            raise ValueError('trial output is not a regular file')
        data = stream.read(ATTACH_LIMIT+1)
    if len(data) >= ATTACH_LIMIT:
        raise ValueError('trial output reached its acceptance limit')
    return data


def logging_config():
    return {'log-driver': 'local', 'log-opts': {
        'max-size': '1m', 'max-file': '2', 'compress': 'false',
        'mode': 'non-blocking', 'max-buffer-size': '1m'}}


def logging_arguments():
    config = logging_config()
    args = ['--log-driver', config['log-driver']]
    for key, value in sorted(config['log-opts'].items()):
        args += ['--log-opt', key+'='+value]
    return args


def require_logging(value):
    config = logging_config()
    if value != {'Type': config['log-driver'], 'Config': config['log-opts']}:
        raise ValueError('disposable container log policy differs')
