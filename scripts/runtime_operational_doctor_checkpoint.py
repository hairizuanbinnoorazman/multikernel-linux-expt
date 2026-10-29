"""Strict version-7 acceptance against independently qualified live views."""
import json
from runtime_doctor_checkpoint import unique_object
from runtime_unified_doctor_checkpoint import qualify as qualify_unified
from runtime_component_build_checkpoint import qualify as qualify_components

UNOBSERVED = ['executable-artifact-identity', 'daemon-process-incarnation', 'component-compatibility',
              'other-component-builds', 'kernel-build-identity', 'kernel-incarnation', 'cpu-transition-health',
              'allocator-internal-metadata', 'native-storage-health', 'native-network-health',
              'network-descriptor-expectations', 'service-readiness', 'docker-engine', 'containment']


def qualify(data, boot, owner, storage_report, network_report, endpoint, combined_report,
            pool_report, unified_report, component_report, versions, revisions, released=False):
    unified=qualify_unified(json.dumps(unified_report).encode(),boot,owner,storage_report,
                            network_report,endpoint,combined_report,pool_report,released=released)
    components=qualify_components(json.dumps(component_report).encode(),boot,storage_report['revision'],versions,revisions)
    if not isinstance(data,bytes) or len(data)>8<<20:
        raise ValueError('version-7 doctor checkpoint exceeds its bound')
    report=json.loads(data,object_pairs_hook=unique_object)
    expected=dict(version=7,scope='resource-network-pool-and-compiled-component-observations',status='consistent',
                  revision=storage_report['revision'],boot_id=boot,boot_stable=True,unified=unified,
                  components=components,findings=[],admission_guaranteed=False,artifact_verified=False,
                  kernel_verified=False,unobserved=UNOBSERVED)
    if json.dumps(report,sort_keys=True)!=json.dumps(expected,sort_keys=True):
        raise ValueError('version-7 doctor differs from qualified resource and compiled-label observations')
    return report
