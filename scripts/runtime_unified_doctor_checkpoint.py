"""Requalify unified doctor against independently checked live component views."""
import json
from runtime_doctor_checkpoint import unique_object
from runtime_combined_doctor_checkpoint import qualify as qualify_combined
from runtime_pool_doctor_checkpoint import qualify as qualify_pool

UNOBSERVED = ['kernel-incarnation', 'cpu-transition-health', 'allocator-internal-metadata',
              'native-storage-health', 'native-network-health', 'network-descriptor-expectations',
              'service-readiness', 'docker-engine', 'containment']


def qualify(data, boot, owner, storage_report, network_report, endpoint,
            combined_report, pool_report, released=False):
    # Independent component commands must themselves qualify. A consistent
    # aggregate label cannot launder an unavailable or mismatched component.
    combined = qualify_combined(json.dumps(combined_report).encode(), boot, owner,
                                storage_report, network_report, endpoint, released=released)
    pool = qualify_pool(json.dumps(pool_report).encode(), boot, owner, storage_report, released=released)
    if not isinstance(data, bytes) or len(data) > 4 << 20:
        raise ValueError('unified doctor checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    expected = dict(version=6, scope='durable-resource-network-native-process-kernel-presence-and-pool-capacity',
                    status='consistent', revision=storage_report['revision'], boot_id=boot, boot_stable=True,
                    combined=combined, pool=pool, findings=[], admission_guaranteed=False, unobserved=UNOBSERVED)
    # Canonical JSON comparison preserves numeric types: Python's True == 1
    # and 0 == 0.0 must never admit forged contract fields/counters/capacity.
    if json.dumps(report, sort_keys=True) != json.dumps(expected, sort_keys=True):
        raise ValueError('unified doctor differs from qualified component observations')
    return report
