"""Strict private-lab correlation of scoped storage and network doctor samples."""
import hashlib
import json
import struct
from runtime_doctor_checkpoint import qualify as qualify_storage, unique_object

UNOBSERVED = ['kernel-incarnation', 'native-network-health',
              'network-descriptor-expectations', 'storage-exports', 'pool-capacity',
              'service-readiness', 'docker-engine', 'containment']


def project_endpoint(endpoint):
    """Independently reproduce the public length-framed descriptor contract."""
    digest = hashlib.sha256(b'mkruntime-network-descriptor-v1\0')
    total = 0

    def number(value):
        if type(value) is not int or not 0 <= value < 1 << 64:
            raise ValueError('invalid endpoint numeric metadata')
        digest.update(struct.pack('>Q', value))

    def string(value):
        nonlocal total
        if not isinstance(value, str):
            raise ValueError('invalid endpoint descriptor string')
        data = value.encode('utf-8')
        total += len(data)
        if len(data) > 4096 or total > 65536:
            raise ValueError('endpoint descriptor exceeds its bound')
        number(len(data))
        digest.update(data)

    for key in ['container_id', 'network_name', 'if_name', 'netns', 'address', 'gateway']:
        string(endpoint[key])
    if type(endpoint['mtu']) is not int or not 576 <= endpoint['mtu'] <= 65535:
        raise ValueError('invalid endpoint MTU')
    number(endpoint['mtu'])
    dns = endpoint['dns']
    string(dns.get('domain', ''))
    for key in ['nameservers', 'search', 'options']:
        values = dns.get(key, [])
        if values is None:
            values = []
        if not isinstance(values, list) or len(values) > 65536:
            raise ValueError('invalid endpoint DNS list')
        number(len(values))
        for value in values:
            string(value)
    counters = {key: endpoint[key] for key in ['rx_packets', 'tx_packets', 'rx_drops', 'tx_drops', 'errors']}
    if any(type(v) is not int or not 0 <= v < 1 << 64 for v in counters.values()):
        raise ValueError('invalid endpoint counters')
    return dict(endpoint_generation=endpoint['generation'], sandbox_id=endpoint['sandbox_id'],
                sandbox_generation=endpoint['sandbox_generation'], primary_boot_id=endpoint['primary_boot_id'],
                owner=endpoint['owner'], managed_namespace=endpoint['managed_namespace'],
                state=endpoint['state'], descriptor_sha256=digest.hexdigest(), counters=counters)


def qualify(data, boot, owner, storage_report, endpoint, released=False):
    # Retain the version-3 contract: the new sample cannot replace storage or
    # native-process inspection with a merely consistent network-only report.
    qualify_storage(json.dumps(storage_report).encode(), boot, owner, released=released)
    if not isinstance(data, bytes) or len(data) > 1 << 20:
        raise ValueError('network doctor checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    fields = {'version', 'scope', 'status', 'boot_id', 'boot_stable', 'revision', 'ownership',
              'kernel_generation_verified', 'findings', 'network', 'network_expectations', 'unobserved'}
    if not isinstance(report, dict) or set(report) not in [fields, fields | {'kernel_instances'}]:
        raise ValueError('invalid network doctor checkpoint fields')
    if (type(report['version']) is not int or report['version'] != 4 or
            report['scope'] != 'durable-lifecycle-network-ownership-and-kernel-presence' or
            report['status'] != 'consistent' or report['boot_id'] != boot or
            report['boot_stable'] is not True or report['kernel_generation_verified'] is not False or
            report['revision'] != storage_report['revision'] or report['findings'] != [] or
            report['unobserved'] != UNOBSERVED or
            json.dumps(report['ownership'], sort_keys=True) != json.dumps(storage_report['ownership'], sort_keys=True) or
            report.get('kernel_instances', []) != storage_report.get('kernel_instances', [])):
        raise ValueError('network and storage doctor observations do not correlate')
    expected_intent = [dict(sandbox_id=v['sandbox_id'], generation=v['generation'], expectation='required')
                       for v in storage_report['ownership']['sandboxes']]
    if report['network_expectations'] != expected_intent:
        raise ValueError('network intent differs from the held child')
    network = report['network']
    if (not isinstance(network, dict) or set(network) != {'version', 'source', 'primary_boot_id', 'endpoints'} or
            type(network['version']) is not int or network['version'] != 1 or
            network['source'] != 'durable-network' or network['primary_boot_id'] != boot):
        raise ValueError('invalid network ownership checkpoint')
    if released:
        if endpoint is not None or network['endpoints'] != []:
            raise ValueError('network endpoints remain after child removal')
    else:
        if (not isinstance(endpoint, dict) or endpoint.get('state') != 'READY' or
                endpoint.get('owner') != 'runtime' or endpoint.get('managed_namespace') is not True or
                endpoint.get('primary_boot_id') != boot or endpoint.get('sandbox_id') != owner['sandbox_id'] or
                endpoint.get('sandbox_generation') != owner['generation']):
            raise ValueError('expected the exact held runtime endpoint')
        if json.dumps(network['endpoints'], sort_keys=True) != json.dumps([project_endpoint(endpoint)], sort_keys=True):
            raise ValueError('network generation, descriptor or counters differ from the held endpoint')
    return report
