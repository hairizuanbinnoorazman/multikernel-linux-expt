"""Strict acceptance of the combined operator command at private live holds."""
import json
from runtime_doctor_checkpoint import unique_object
from runtime_network_doctor_checkpoint import qualify as qualify_network

UNOBSERVED = ['kernel-incarnation', 'native-storage-health', 'native-network-health',
              'network-descriptor-expectations', 'pool-capacity', 'service-readiness',
              'docker-engine', 'containment']


def qualify(data, boot, owner, storage_report, network_report, endpoint, released=False):
    # Requalify both inputs, rather than trusting a consistent label or accepting
    # a union of unrelated successful reports from different owners or samples.
    qualify_network(json.dumps(network_report).encode(), boot, owner, storage_report, endpoint, released=released)
    if not isinstance(data, bytes) or len(data) > 2 << 20:
        raise ValueError('combined doctor checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    expected = dict(storage_report, version=5,
                    scope='durable-resource-network-ownership-native-process-identity-and-kernel-presence',
                    unobserved=UNOBSERVED, network=network_report['network'],
                    network_expectations=network_report['network_expectations'])
    if json.dumps(report, sort_keys=True) != json.dumps(expected, sort_keys=True):
        raise ValueError('combined doctor differs from qualified storage/network observations')
    return report
