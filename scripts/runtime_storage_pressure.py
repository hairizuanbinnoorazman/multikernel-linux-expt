"""Strict interpretation of the bounded guest filesystem exhaustion fixture."""
import json
import re


def field(text, name):
    prefix = 'G8_STORAGE_PRESSURE_' + name + '='
    values = [line[len(prefix):] for line in text.splitlines() if line.startswith(prefix)]
    if len(values) != 1:
        raise ValueError('missing or ambiguous storage pressure measurement')
    return values[0]


def number(text, name):
    value = field(text, name)
    if not re.fullmatch('0|[1-9][0-9]*', value):
        raise ValueError('noncanonical storage pressure measurement')
    return int(value)


def full_observation(stdout, stderr):
    before = number(stdout, 'BEFORE_KIB')
    full = number(stdout, 'FULL_KIB')
    size = number(stdout, 'FILE_BYTES')
    status = number(stdout, 'WRITE_EXIT')
    if (before <= 65536 or full != 0 or not 64*1024*1024 < size <= 2*1024**3 or
            status != 1 or number(stdout, 'SYNC_EXIT') != 0 or 'No space left on device' not in stderr or
            field(stdout, 'FS_MAGIC') != 'ef53' or
            'G8_STORAGE_PRESSURE_FULL\n' not in stdout):
        raise ValueError('guest did not prove the expected bounded ENOSPC condition')
    return {'before_available_kib': before, 'full_available_kib': full,
            'pressure_file_bytes': size, 'write_exit_status': status}


def recovered_observation(stdout, stderr):
    result = full_observation(stdout, stderr)
    restored = number(stdout, 'RESTORED_KIB')
    if restored < max(65536, result['before_available_kib'] - 2048):
        raise ValueError('guest space was not restored after removing the pressure file')
    if 'G8_STORAGE_PRESSURE_BUILD_RECOVERED\n' not in stdout or 'G8_BUILD_WORKLOAD_PASS\n' not in stdout:
        raise ValueError('post-pressure build matrix did not pass')
    engines = [json.loads(line) for line in stdout.splitlines()
               if line.startswith('{') and '"KernelVersion"' in line]
    if len(engines) != 2 or not engines[0].get('ID') or engines[0]['ID'] != engines[1]['ID']:
        raise ValueError('guest engine identity changed across filesystem pressure')
    for engine in engines:
        if (engine['ServerVersion'] != '29.9.0' or engine['Driver'] != 'vfs' or
                'name=rootless' not in engine['SecurityOptions'] or
                engine['DockerRootDir'] != '/home/rootless/.local/share/docker' or
                engine['KernelVersion'] != engines[0]['KernelVersion']):
            raise ValueError('post-pressure engine does not match the approved guest')
    return {**result, 'restored_available_kib': restored, 'engine_id': engines[0]['ID'],
            'kernel_release': engines[0]['KernelVersion']}
