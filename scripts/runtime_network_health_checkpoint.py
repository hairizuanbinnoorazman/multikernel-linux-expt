"""Bind backend health observations to independently qualified Docker ownership."""
import json
from runtime_doctor_checkpoint import unique_object
from runtime_network_doctor_checkpoint import qualify as qualify_network


def qualify(data, boot, owner, storage_report, network_report, endpoint, released=False):
    # Requalify the complete independent reference chain, including storage and
    # exact endpoint generation/descriptor. Internally consistent forgery alone
    # must not turn a health response into a qualified active Docker observation.
    qualify_network(json.dumps(network_report).encode(), boot, owner,
                    storage_report, endpoint, released=released)
    if not isinstance(data, bytes) or not 0 < len(data) <= 64 << 10:
        raise ValueError('network health checkpoint exceeds its bound')
    report = json.loads(data, object_pairs_hook=unique_object)
    if (not isinstance(report, dict) or
            set(report) != {'version', 'source', 'primary_boot_id', 'status', 'endpoints'} or
            type(report['version']) is not int or report['version'] != 1 or
            report['source'] != 'host-network-backend-check' or
            report['primary_boot_id'] != boot or
            report['status'] != ('idle' if released else 'checked')):
        raise ValueError('invalid network health checkpoint')
    expected = [dict(endpoint_generation=v['endpoint_generation'],
                     descriptor_sha256=v['descriptor_sha256'], code='CHECK_PASSED')
                for v in network_report['network']['endpoints']]
    if report['endpoints'] != expected:
        raise ValueError('network health generation, descriptor or check differs')
    return report
