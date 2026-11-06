"""Qualify a fixture's unit observation without crossing a boot epoch.

A completed unit is only process-status evidence. It never proves resource
cleanup, backing durability, recovery or a full gate. Inputs come from pinned
receipts and bounded read-only observations; errors never echo their contents.
"""
import re

_BOOT = re.compile(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}')
_INVOCATION = re.compile(r'[a-f0-9]{32}')
_FIELDS = {'MainPID', 'InvocationID', 'SubState', 'Result', 'ExecMainStatus', 'ControlGroup'}


def unit_properties(text):
    """Parse exactly the six requested systemctl properties, without aliases."""
    if type(text) is not str or len(text) > 8192:
        raise ValueError('invalid fixture unit observation')
    values = {}
    for line in text.splitlines():
        key, separator, value = line.partition('=')
        if not separator or key not in _FIELDS or key in values or any(ord(c) < 32 or ord(c) > 126 for c in value):
            raise ValueError('invalid fixture unit observation')
        values[key] = value
    _validate_unit(values)
    return values


def _validate_unit(unit):
    if type(unit) is not dict or set(unit) != _FIELDS or any(type(v) is not str or len(v) > 4096 for v in unit.values()):
        raise ValueError('invalid fixture unit observation')
    for name, maximum in [('MainPID', (1 << 31) - 1), ('ExecMainStatus', 255)]:
        value = unit[name]
        if not re.fullmatch(r'0|[1-9][0-9]{0,9}', value) or int(value) > maximum:
            raise ValueError('invalid fixture unit observation')
    if unit['InvocationID'] and not _INVOCATION.fullmatch(unit['InvocationID']):
        raise ValueError('invalid fixture unit observation')
    if unit['SubState'] not in {'dead', 'exited', 'running', 'failed', 'start', 'stop', 'auto-restart'} or unit['Result'] not in {'success', 'exit-code', 'signal', 'timeout', 'resources', 'core-dump', 'start-limit-hit'}:
        raise ValueError('invalid fixture unit observation')
    if unit['ControlGroup'] and (not unit['ControlGroup'].startswith('/') or any(ord(c) < 32 or ord(c) > 126 for c in unit['ControlGroup'])):
        raise ValueError('invalid fixture unit observation')


def qualify_unit_epoch(receipt_boot, receipt_invocation, observed_boot, unit):
    if any(type(v) is not str or not _BOOT.fullmatch(v) for v in [receipt_boot, observed_boot]) or type(receipt_invocation) is not str or not _INVOCATION.fullmatch(receipt_invocation):
        raise ValueError('invalid fixture epoch identity')
    _validate_unit(unit)
    same_boot = receipt_boot == observed_boot
    invocation_matches = unit['InvocationID'] == receipt_invocation
    complete = same_boot and invocation_matches and unit['MainPID'] == '0' and unit['SubState'] == 'exited' and unit['Result'] == 'success' and unit['ExecMainStatus'] == '0'
    if not same_boot:
        code = 'BOOT_CHANGED'
    elif not invocation_matches:
        code = 'UNIT_UNAVAILABLE_OR_REPLACED'
    elif not complete:
        code = 'UNIT_NOT_SUCCESSFULLY_EXITED'
    else:
        code = 'SAME_BOOT_COMPLETED_UNIT'
    return dict(same_boot=same_boot, invocation_matches=invocation_matches,
                unit_completion_qualified=complete, code=code)
