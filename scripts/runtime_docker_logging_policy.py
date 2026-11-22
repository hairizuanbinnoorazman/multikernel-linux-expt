"""Explicit bounded Docker logging policy shared by deployment and trials."""

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
