"""Strict acceptance of the scoped live doctor's presence checkpoints."""
import json
import re
import uuid

UNOBSERVED = ['kernel-incarnation', 'storage-exports', 'network-endpoints',
              'pool-capacity', 'service-readiness', 'docker-engine', 'containment']


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('ambiguous doctor checkpoint')
        result[key] = value
    return result


def qualify(data, boot, owner, released=False):
    if not isinstance(data, bytes) or len(data) > 1 << 20:
        raise ValueError('doctor checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    expected = {'version', 'scope', 'status', 'boot_id', 'boot_stable', 'revision',
                'ownership', 'kernel_generation_verified', 'findings', 'unobserved'}
    if not isinstance(report, dict) or set(report) not in [expected, expected | {'kernel_instances'}]:
        raise ValueError('invalid doctor checkpoint fields')
    if (str(uuid.UUID(boot)) != boot or report['boot_id'] != boot or
            type(report['version']) is not int or report['version'] != 1 or
            report['scope'] != 'durable-ownership-and-kernel-presence' or
            report['status'] != 'consistent' or report['boot_stable'] is not True or
            report['kernel_generation_verified'] is not False or
            report['findings'] != [] or report['unobserved'] != UNOBSERVED or
            not isinstance(report['revision'], str) or
            not re.fullmatch(r'[0-9a-f]{7,40}', report['revision'])):
        raise ValueError('doctor checkpoint is not a stable scoped observation')
    durable = report['ownership']
    if (not isinstance(durable, dict) or set(durable) != {'version', 'source', 'sequence', 'sandboxes'} or
            type(durable['version']) is not int or durable['version'] != 1 or
            durable['source'] != 'durable-lifecycle' or
            type(durable['sequence']) is not int or not 0 <= durable['sequence'] < 1 << 64):
        raise ValueError('invalid doctor ownership checkpoint')
    if owner.get('state') != 'RUNNING':
        raise ValueError('expected the held running owner')
    if released:
        allowed = [[], [{**owner, 'state': 'ABSENT'}]]
        if durable['sandboxes'] not in allowed or report.get('kernel_instances', []) != []:
            raise ValueError('doctor still observes child ownership after removal')
    elif durable['sandboxes'] != [owner] or report.get('kernel_instances') != [owner['sandbox_id']]:
        raise ValueError('doctor ownership or kernel name differs from the held child')
    return report
