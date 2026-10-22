"""Compare iptables-save rule identity while excluding volatile chain counters."""
import re


def identity(raw):
    lines = []
    for line in raw.splitlines():
        if line.startswith(b'#'):
            continue
        if line.startswith(b':'):
            match = re.fullmatch(rb'(:\S+ (?:ACCEPT|DROP|-)) \[[0-9]+:[0-9]+\]', line)
            if not match:
                raise ValueError('unrecognized firewall chain declaration')
            line = match[1]
        lines.append(line)
    return b'\n'.join(lines) + b'\n'
