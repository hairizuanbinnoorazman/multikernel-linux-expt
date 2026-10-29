"""Strict private-lab acceptance of compiled component metadata, not health."""
import json
from runtime_doctor_checkpoint import unique_object

UNOBSERVED = ['executable-artifact-identity', 'daemon-process-incarnation', 'component-compatibility',
              'other-component-builds', 'kernel-build-identity', 'kernel-incarnation', 'service-readiness',
              'native-storage-health', 'native-network-health', 'docker-engine', 'containment']


def expected_base(boot, revision):
    return dict(version=1,scope='compiled-runtime-network-build-self-reports',status='inconclusive',
                revision=revision,boot_id=boot,boot_stable=False,compiled_revisions_match=False,
                findings=[],artifact_verified=False,kernel_verified=False,unobserved=UNOBSERVED)


def decode(data):
    if not isinstance(data, bytes) or len(data) > 65536:
        raise ValueError('component build checkpoint exceeds its bound')
    return json.loads(data, object_pairs_hook=unique_object)


def equal(actual, expected):
    if json.dumps(actual, sort_keys=True) != json.dumps(expected, sort_keys=True):
        raise ValueError('component build checkpoint differs from independently known compiled labels')
    return actual


def qualify(data, boot, inspector_revision, versions, revisions):
    expected=expected_base(boot,inspector_revision)
    expected.update(status='consistent',boot_stable=True,compiled_revisions_match=revisions['mkruntimed']==revisions['mknetd'])
    for field,component in [('runtime','mkruntimed'),('network','mknetd')]:
        if revisions[component]=='unknown':
            raise ValueError('known-label checkpoint cannot qualify unknown revision')
        expected[field]=dict(version=1,source='compiled-component-self-report',component=component,
                             build_version=versions[component],build_revision=revisions[component],
                             artifact_verified=False,kernel_verified=False)
    return equal(decode(data),expected)


def qualify_unsupported(data, boot, inspector_revision, component):
    if component not in ['RUNTIME','NETWORK']:
        raise ValueError('invalid unsupported component')
    expected=expected_base(boot,inspector_revision)
    expected['findings']=[dict(code=component+'_BUILD_UNSUPPORTED')]
    return equal(decode(data),expected)
