"""Project systemd capability/NNP policy into disposable service qualification."""
import re


def parse_policy(text, capability_names):
    if len(text) > 65536 or not 1 <= len(capability_names) <= 64:
        raise ValueError('service policy exceeds qualification bounds')
    if len(set(capability_names)) != len(capability_names):
        raise ValueError('duplicate capability ABI name')
    if any(not re.fullmatch('[a-z][a-z0-9_]*', name) for name in capability_names):
        raise ValueError('invalid capability ABI name')
    selected = {}
    service = False
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith(('#', ';')):
            continue
        if line.startswith('['):
            service = line == '[Service]'
            continue
        if not service or '=' not in line:
            continue
        key, value = line.split('=', 1)
        if key not in {'CapabilityBoundingSet', 'NoNewPrivileges', 'AmbientCapabilities'}:
            continue
        if key in selected:
            raise ValueError('ambiguous repeated service restriction')
        selected[key] = value.strip()
    if selected.get('NoNewPrivileges') != 'yes' or selected.get('AmbientCapabilities') != '':
        raise ValueError('qualification requires NNP and empty ambient capabilities')
    caps = selected.get('CapabilityBoundingSet', '').split()
    if not caps or len(caps) != len(set(caps)):
        raise ValueError('explicit unique service capabilities required')
    names = []
    for cap in caps:
        if not cap.startswith('CAP_') or cap[4:].lower() not in capability_names:
            raise ValueError('unknown or noncanonical service capability')
        name = cap[4:].lower()
        if cap != 'CAP_' + name.upper():
            raise ValueError('noncanonical service capability')
        names.append(name)
    mask = sum(1 << capability_names.index(name) for name in names)
    return {'capabilities': names, 'expected_capability_mask': f'{mask:016x}',
            'no_new_privileges': True, 'ambient_capabilities': []}


def command_with_policy(command, policy):
    return ['/usr/bin/setpriv', '--no-new-privs', '--inh-caps=-all',
            '--ambient-caps=-all', '--bounding-set=-all,' +
            ','.join('+' + name for name in policy['capabilities'])] + list(command)


def check_observation(observed, policy):
    for key in ['CapBnd', 'CapEff', 'CapPrm']:
        if observed.get(key) != policy['expected_capability_mask']:
            raise ValueError('observed service capabilities differ from declared policy')
    for key in ['CapInh', 'CapAmb']:
        if observed.get(key) != '0000000000000000':
            raise ValueError('unexpected inherited or ambient service capabilities')
    if observed.get('NoNewPrivs') != '1' or observed.get('Uid', '').split() != ['0'] * 4:
        raise ValueError('observed service NNP/identity differs from declared policy')
