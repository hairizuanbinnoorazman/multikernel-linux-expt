"""Strict acceptance of the scoped live doctor's presence checkpoints."""
import json
import re
import uuid

UNOBSERVED = ['kernel-incarnation', 'native-storage-health', 'network-endpoints',
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
                'ownership', 'resources', 'native_processes', 'kernel_generation_verified', 'findings', 'unobserved'}
    if not isinstance(report, dict) or set(report) not in [expected, expected | {'kernel_instances'}]:
        raise ValueError('invalid doctor checkpoint fields')
    if (str(uuid.UUID(boot)) != boot or report['boot_id'] != boot or
            type(report['version']) is not int or report['version'] != 3 or
            report['scope'] != 'durable-resource-ownership-native-process-identity-and-kernel-presence' or
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
    resources = report['resources']
    if (not isinstance(resources, dict) or set(resources) != {'version', 'source', 'lifecycle', 'storage_bindings', 'storage', 'storage_descriptors'} or
            type(resources['version']) is not int or resources['version'] != 1 or
            resources['source'] != 'durable-lifecycle-storage' or resources['lifecycle'] != durable):
        raise ValueError('invalid combined resource checkpoint')
    store = resources['storage']
    if (not isinstance(store, dict) or set(store) != {'version', 'source', 'exports'} or
            type(store['version']) is not int or store['version'] != 1 or
            store['source'] != 'durable-storage' or not isinstance(store['exports'], list) or len(store['exports']) != 1):
        raise ValueError('expected one generation-bound storage lease')
    lease = store['exports'][0]
    fields = {'sandbox_id', 'sandbox_generation', 'export_generation', 'peer_cid', 'state', 'filesystem_uuid', 'size_bytes', 'quota_bytes', 'inode_limit', 'port', 'image_device', 'image_inode', 'offline_check', 'counters'}
    if (not isinstance(lease, dict) or set(lease) != fields or lease['sandbox_id'] != owner['sandbox_id'] or
            lease['sandbox_generation'] != owner['generation'] or lease['peer_cid'] != owner['child_cid'] or
            not isinstance(lease['export_generation'], str) or not re.fullmatch('[0-9a-f]{32}', lease['export_generation'])):
        raise ValueError('storage lease belongs to another child or generation')
    native = report['native_processes']
    expected_process = dict(sandbox_id=lease['sandbox_id'], generation=lease['sandbox_generation'],
                            export_generation=lease['export_generation'],
                            status='not-probed-released' if released else 'process-matched')
    if native != [expected_process]:
        raise ValueError('native process identity is missing, unobserved or belongs to another generation')
    for key in ['peer_cid', 'size_bytes', 'quota_bytes', 'inode_limit', 'port', 'image_device', 'image_inode']:
        if type(lease[key]) is not int or not 0 < lease[key] < 1 << 64:
            raise ValueError('invalid storage numeric metadata')
    if (not 64 << 20 <= lease['size_bytes'] <= 16 << 30 or lease['size_bytes'] % 4096 or
            lease['quota_bytes'] != lease['size_bytes'] or not 128 <= lease['inode_limit'] <= 2097152 or
            not 1024 <= lease['port'] <= 65535 or not isinstance(lease['filesystem_uuid'], str) or
            not re.fullmatch('[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}', lease['filesystem_uuid'])):
        raise ValueError('invalid storage descriptor metadata')
    counters = lease['counters']
    if (not isinstance(counters, dict) or set(counters) != {'reads', 'read_bytes', 'writes', 'written_bytes', 'flushes'} or
            any(type(v) is not int or not 0 <= v < 1 << 64 for v in counters.values())):
        raise ValueError('invalid recorded close counters')
    if released:
        if lease['state'] != 'RELEASED' or not re.fullmatch('e2fsck-clean-sha256:[0-9a-f]{64}', lease['offline_check']):
            raise ValueError('storage release is not recorded cleanly')
    elif lease['state'] != 'ACTIVE' or lease['offline_check'] != '' or any(counters.values()):
        raise ValueError('storage lease is not recorded active')
    descriptors = resources['storage_descriptors']
    if not isinstance(descriptors, list) or len(descriptors) != 1:
        raise ValueError('missing storage descriptor fingerprint')
    descriptor = descriptors[0]
    if (not isinstance(descriptor, dict) or set(descriptor) != {'sandbox_id', 'generation', 'export_generation', 'descriptor_sha256'} or
            descriptor['sandbox_id'] != owner['sandbox_id'] or descriptor['generation'] != owner['generation'] or
            descriptor['export_generation'] != lease['export_generation'] or not isinstance(descriptor['descriptor_sha256'], str) or
            not re.fullmatch('[0-9a-f]{64}', descriptor['descriptor_sha256'])):
        raise ValueError('invalid storage descriptor fingerprint')
    expected_binding = dict(descriptor, required=True)
    expected = [] if durable['sandboxes'] == [] else [expected_binding]
    if (resources['storage_bindings'] != expected or
            any(binding.get('required') is not True for binding in resources['storage_bindings'])):
        raise ValueError('lifecycle expectation differs from the storage descriptor')
    return report


def _qualify_record_fault(data, boot, owner, baseline, native_status, finding_code, report_status):
    """Accept only the exact degraded counterpart of a qualified held report."""
    if not isinstance(data, bytes) or len(data) > 1 << 20:
        raise ValueError('doctor checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    if not isinstance(report, dict):
        raise ValueError('invalid missing-record checkpoint')
    expected_process = dict(baseline['native_processes'][0], status=native_status)
    finding = dict(code=finding_code, sandbox_id=owner['sandbox_id'],
                   generation=owner['generation'], export_generation=expected_process['export_generation'])
    if (report.get('status') != report_status or report.get('findings') != [finding] or
            report.get('native_processes') != [expected_process]):
        raise ValueError('expected exact generation-bound native-record fault finding')
    normalized = dict(report, status='consistent', findings=[], native_processes=baseline['native_processes'])
    qualified = qualify(json.dumps(normalized).encode(), boot, owner)
    if qualified != qualify(json.dumps(baseline).encode(), boot, owner):
        raise ValueError('recorded claims changed during missing-record inspection')
    return report


def qualify_missing_record(data, boot, owner, baseline):
    return _qualify_record_fault(data, boot, owner, baseline, 'record-missing',
                                'NATIVE_STORAGE_RECORD_MISSING', 'degraded')


def qualify_unavailable_record(data, boot, owner, baseline):
    return _qualify_record_fault(data, boot, owner, baseline, 'unavailable',
                                'NATIVE_STORAGE_IDENTITY_UNAVAILABLE', 'inconclusive')


def qualify_absent_process(data, boot, owner, baseline):
    return _qualify_record_fault(data, boot, owner, baseline, 'process-absent',
                                'NATIVE_STORAGE_PROCESS_ABSENT', 'degraded')
