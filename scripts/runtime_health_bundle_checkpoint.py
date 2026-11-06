"""Exact steady Docker-hold acceptance for v8 health and v2 private bundles.

Health is correlated observation, never readiness, writability or admission.
All independent v7/journal/network references are requalified before binding.
"""
import hashlib
import json
import re
from runtime_diagnostic_bundle_checkpoint import decode, equal, EXCLUDED, read_fixture_artifact
from runtime_operational_doctor_checkpoint import qualify as qualify_operational
from runtime_journal_status_checkpoint import qualify as qualify_journal
from runtime_network_health_checkpoint import qualify as qualify_network

SCOPE = 'operational-health-and-component-observation-snapshot'
UNOBSERVED = ['executable-artifact-identity', 'daemon-process-incarnation', 'component-compatibility',
              'kernel-build-identity', 'kernel-incarnation', 'cpu-transition-health', 'allocator-internal-metadata',
              'native-storage-health', 'end-to-end-network-health', 'journal-current-writability',
              'journal-byte-integrity', 'service-readiness', 'docker-engine', 'containment']


def qualify(data, boot, owner, storage_report, network_report, endpoint, combined_report, pool_report,
            unified_report, component_report, operational_report, journal_report, health_report,
            versions, revisions, released=False):
    operational = qualify_operational(json.dumps(operational_report).encode(), boot, owner, storage_report,
                                     network_report, endpoint, combined_report, pool_report, unified_report,
                                     component_report, versions, revisions, released=released)
    journal = qualify_journal(json.dumps(journal_report).encode(), boot, owner, storage_report, released=released)
    network = qualify_network(json.dumps(health_report).encode(), boot, owner, storage_report, network_report,
                              endpoint, released=released)
    report = decode(data, 8 << 20)
    expected = dict(version=8, scope='operational-journal-and-recorded-network-health-observations',
                    status='consistent', revision=storage_report['revision'], boot_id=boot, boot_stable=True,
                    operational=operational, journal=journal, network_health=network, findings=[],
                    admission_guaranteed=False, unobserved=UNOBSERVED)
    equal(report, expected)
    return report


def qualify_bundle(data, receipt_data, health_data, *references, **options):
    health = qualify(health_data, *references, **options)
    receipt = decode(receipt_data, 65536)
    equal(receipt, dict(version=2, scope=SCOPE, collection_status='captured', observation_status='consistent',
                        bytes=len(data), sha256=hashlib.sha256(data).hexdigest()))
    bundle = decode(data, 8 << 20)
    if not isinstance(bundle, dict):
        raise ValueError('invalid health bundle envelope')
    for name in ['started_at', 'finished_at']:
        value = bundle.get(name)
        if not isinstance(value, str) or not re.fullmatch(r'[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z', value):
            raise ValueError('invalid health bundle timestamp')
    equal(bundle, dict(version=2, scope=SCOPE, collection_status='captured', started_at=bundle['started_at'],
                       finished_at=bundle['finished_at'], snapshot=dict(components=health['operational']['components'],
                       health=health), excluded=EXCLUDED))
    return bundle


def qualify_timing(data, phase):
    timing = decode(data, 65536)
    keys = {'version', 'scope', 'phase', 'configured_budget_ns', 'started_monotonic_ns', 'deadline_monotonic_ns',
            'finished_monotonic_ns', 'elapsed_ns', 'report_count'}
    if type(timing) is not dict or set(timing) != keys or phase not in ['while-full', 'while-image-ready', 'after-remove', 'record-restored']:
        raise ValueError('invalid health checkpoint timing')
    if any(type(timing[k]) is not int for k in keys-{'scope', 'phase'}):
        raise ValueError('invalid health checkpoint timing number')
    if (timing['version'] != 1 or timing['scope'] != 'health-doctor-and-health-bundle-checkpoint' or
            timing['phase'] != phase or timing['configured_budget_ns'] != 5_000_000_000 or timing['report_count'] != 2):
        raise ValueError('invalid health checkpoint timing scope')
    start, end, deadline = [timing[k] for k in ['started_monotonic_ns', 'finished_monotonic_ns', 'deadline_monotonic_ns']]
    if not 0 <= start <= end <= deadline <= start+timing['configured_budget_ns'] or timing['elapsed_ns'] != end-start:
        raise ValueError('health checkpoint exceeded its shared budget')
    return timing


def inspect_fixture_health(work, boot, invocation):
    """Bind private health frames/products to the exact completed workload.

    Requalify independent references with the endpoint retained at the hold.
    Raw state, endpoint descriptors and product contents remain private.
    """
    import os
    from pathlib import Path
    work = Path(work)
    phases = ['while-full', 'while-image-ready', 'after-remove']
    private = work/'integration'
    receipt = decode(read_fixture_artifact(work, 'integration-receipt.json', 1 << 20), 1 << 20)
    if (not re.fullmatch(r'[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}', boot) or
            not re.fullmatch('[a-f0-9]{32}', invocation) or receipt.get('boot_id') != boot or
            receipt.get('invocation_id') != invocation or receipt.get('runtime_health_bundle_tested') is not True or
            receipt.get('production_profile_child_build_pass') is not True or receipt.get('storage_pressure_tested') is not True):
        raise ValueError('health lifecycle receipt identity differs')
    def read(parent, name):
        return read_fixture_artifact(parent, name, 1 << 20)
    def report(prefix, phase):
        return decode(read(private, prefix+'-'+phase+'.json'), 1 << 20)
    owner = decode(read(work, 'storage-pressure-full-checkpoint.json'), 1 << 20)['owner']
    labels = decode(read(work, 'compiled-labels.json'), 1 << 20)
    timings = {}
    for phase in phases:
        values = {}
        for family, parent, name in [
            ('doctor', private, 'health-doctor-'+phase+'.json'),
            ('endpoint', private, 'health-endpoint-'+phase+'.json'),
            ('bundle', private/'health-bundles', phase+'.json'),
            ('bundle_receipt', private, 'health-bundle-'+phase+'-receipt.json'),
            ('bundle_identity', private, 'health-bundle-'+phase+'-identity.json'),
            ('checkpoint_timing', private, 'health-checkpoint-timing-'+phase+'.json')]:
            hashes = receipt.get('runtime_health_'+family+'_sha256')
            if type(hashes) is not dict or set(hashes) != set(phases):
                raise ValueError('incomplete health lifecycle evidence')
            data = read(parent, name)
            if hashlib.sha256(data).hexdigest() != hashes[phase]:
                raise ValueError('health lifecycle receipt digest differs')
            values[family] = data
        endpoint = decode(values['endpoint'], 1 << 20)
        references = (boot, owner, report('doctor', phase), report('network-doctor', phase), endpoint,
                      report('combined-doctor', phase), report('pool-doctor', phase), report('unified-doctor', phase),
                      report('component-builds', phase), report('operational-doctor', phase),
                      report('journal-status', phase), report('network-health', phase), labels['versions'], labels['revisions'])
        qualify_bundle(values['bundle'], values['bundle_receipt'], values['doctor'], *references, released=phase=='after-remove')
        info = (private/'health-bundles'/(phase+'.json')).lstat()
        import stat
        if not stat.S_ISREG(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o600 or info.st_uid != os.geteuid() or info.st_nlink != 1:
            raise ValueError('health bundle identity differs')
        equal(decode(values['bundle_identity'], 65536), dict(device=info.st_dev, inode=info.st_ino, uid=info.st_uid,
              links=info.st_nlink, mode='0600', bytes=info.st_size, sha256=hashlib.sha256(values['bundle']).hexdigest()))
        timings[phase] = qualify_timing(values['checkpoint_timing'], phase)
    for first, second in zip(phases, phases[1:]):
        if timings[first]['finished_monotonic_ns'] > timings[second]['started_monotonic_ns']:
            raise ValueError('health lifecycle phase order differs')
    return dict(version=1, scope='receipt-bound-health-doctor-and-bundle-checkpoints', boot_id=boot,
                invocation_id=invocation, timings=timings, maximum_elapsed_ns=max(t['elapsed_ns'] for t in timings.values()))
