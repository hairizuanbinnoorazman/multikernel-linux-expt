#!/usr/bin/env python3
"""Independent reference, budget and receipt-rebinding controls for v9/v3."""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
from types import SimpleNamespace

import runtime_aggregate_docker_health_checkpoint as subject
from runtime_diagnostic_bundle_checkpoint import EXCLUDED
from runtime_docker_concurrency import sandbox_id_for_container

spec = importlib.util.spec_from_file_location('health_controls', Path(__file__).with_name('test-runtime-health-bundle-checkpoint.py'))
health_controls = importlib.util.module_from_spec(spec)
spec.loader.exec_module(health_controls)
CONTAINER = 'a'*64
SID = sandbox_id_for_container(CONTAINER)
BOOT = health_controls.controls.BOOT
INV = '9'*32
encode = lambda v: json.dumps(v).encode()
sha = lambda data: hashlib.sha256(data).hexdigest()


class AggregateCheckpoint(unittest.TestCase):
    def setUp(self):
        self.umask = os.umask(0o077)

    def tearDown(self):
        os.umask(self.umask)

    def fixture(self, released=False):
        args, health, _ = json.loads(json.dumps(health_controls.HealthCheckpointTests().fixture(released)).replace('"box-a"', json.dumps(SID)))
        engines = [] if released else [dict(sandbox_id=SID, generation=args[1]['generation'],
            observation=dict(version=1, source='owned-child-docker-api-and-launcher', status='responding'))]
        doctor = dict(version=9, scope=subject.SCOPE, status='consistent', revision=health['revision'],
            boot_id=BOOT, boot_stable=True, health=health, engines=engines, all_running_owners_observed=True,
            findings=[], admission_guaranteed=False, unobserved=[v for v in health['unobserved'] if v != 'docker-engine'])
        bundle = dict(version=3, scope=subject.BUNDLE_SCOPE, collection_status='captured',
            started_at='2026-11-09T00:00:00Z', finished_at='2026-11-09T00:00:01Z',
            snapshot=dict(components=args[8], docker_health=doctor), excluded=EXCLUDED)
        return args, doctor, bundle

    def ack(self, data, version=3, scope=subject.BUNDLE_SCOPE):
        return dict(version=version, scope=scope, collection_status='captured', observation_status='consistent',
                    bytes=len(data), sha256=sha(data))

    def test_live_and_released_exact_independent_health_and_owner_references(self):
        for released in [False, True]:
            args, doctor, bundle = self.fixture(released)
            self.assertEqual(subject.qualify(encode(doctor), *args, released=released), doctor)
            data = encode(bundle)
            self.assertEqual(subject.qualify_bundle(data, encode(self.ack(data)), encode(doctor), *args, released=released), bundle)

    def test_forged_coverage_nested_health_private_data_and_rebound_receipts_refuse(self):
        variants = ['version', 'numeric-alias', 'coverage', 'missing-engine', 'extra-engine', 'generation', 'sandbox',
                    'source', 'status', 'private', 'admission', 'unobserved', 'health-sequence', 'health-network',
                    'component', 'bundle-version', 'bundle-scope', 'bundle-private', 'released-engine', 'bundle-doctor-drift']
        for variant in variants:
            released = variant == 'released-engine'
            args, doctor, bundle = self.fixture(released)
            if variant == 'version': doctor['version'] = 8
            elif variant == 'numeric-alias': doctor['version'] = 9.0
            elif variant == 'coverage': doctor['all_running_owners_observed'] = 1
            elif variant == 'missing-engine': doctor['engines'] = []
            elif variant == 'extra-engine': doctor['engines'].append(copy.deepcopy(doctor['engines'][0]))
            elif variant == 'generation': doctor['engines'][0]['generation'] = 'f'*32
            elif variant == 'sandbox': doctor['engines'][0]['sandbox_id'] = 'sibling'
            elif variant == 'source': doctor['engines'][0]['observation']['source'] = 'other'
            elif variant == 'status': doctor['engines'][0]['observation']['status'] = 'unavailable'
            elif variant == 'private': doctor['socket'] = '/private/synthetic-canary'
            elif variant == 'admission': doctor['admission_guaranteed'] = True
            elif variant == 'unobserved': doctor['unobserved'] = []
            elif variant == 'health-sequence': doctor['health']['journal']['sequence'] += 1
            elif variant == 'health-network': doctor['health']['network_health']['status'] = 'degraded'
            elif variant == 'component': bundle['snapshot']['components'] = {}
            elif variant == 'bundle-version': bundle['version'] = 2
            elif variant == 'bundle-scope': bundle['scope'] = 'readiness'
            elif variant == 'bundle-private': bundle['raw_logs'] = 'synthetic-canary'
            elif variant == 'released-engine': doctor['engines'] = self.fixture()[1]['engines']
            elif variant == 'bundle-doctor-drift':
                bundle = copy.deepcopy(bundle); bundle['snapshot']['docker_health']['engines'] = []
            data = encode(bundle)
            with self.subTest(variant=variant), self.assertRaises(ValueError):
                subject.qualify_bundle(data, encode(self.ack(data)), encode(doctor), *args, released=released)
        args, doctor, _ = self.fixture()
        with self.assertRaises(ValueError):
            subject.qualify(b'{"version":9,'+encode(doctor)[1:], *args)

    def timing(self, phase='while-full', start=100):
        pair = dict(version=1, scope='health-doctor-and-health-bundle-checkpoint', phase=phase,
            configured_budget_ns=5_000_000_000, started_monotonic_ns=start, deadline_monotonic_ns=start+500,
            finished_monotonic_ns=start+100, elapsed_ns=100, report_count=2)
        timing = dict(version=1, scope=subject.TIMING_SCOPE, phase=phase, configured_budget_ns=5_000_000_000,
            started_monotonic_ns=start, deadline_monotonic_ns=start+500, aggregate_started_monotonic_ns=start+100,
            finished_monotonic_ns=start+200, elapsed_ns=200, report_count=4)
        return pair, timing

    def test_original_pair_budget_cannot_be_extended_restarted_or_forged(self):
        pair, timing = self.timing()
        self.assertEqual(subject.qualify_timing(encode(timing), 'while-full', pair), timing)
        for key, value in [('deadline_monotonic_ns', 601), ('started_monotonic_ns', 101), ('report_count', 4.0),
                           ('aggregate_started_monotonic_ns', 199), ('finished_monotonic_ns', 601),
                           ('configured_budget_ns', 6_000_000_000), ('elapsed_ns', 201), ('version', True)]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                subject.qualify_timing(encode(dict(timing, **{key: value})), 'while-full', pair)
        with self.assertRaises(ValueError):
            subject.qualify_timing(encode(timing), 'while-full', dict(pair, report_count=4))

    def test_target_map_exact_owner_and_canonical_private_socket(self):
        args, _, _ = self.fixture()
        owner = args[1]
        target = subject.targets(CONTAINER, owner, 'unix:///private/shim.sock')
        self.assertEqual(target['targets'][0]['owner']['sandbox_id'], SID)
        self.assertEqual(subject.targets(CONTAINER, owner, 'unix:///private/shim.sock', True), dict(version=1, targets=[]))
        for container, candidate, address in [('b'*64, owner, 'unix:///private/shim.sock'),
                (CONTAINER, dict(owner, generation='bad'), 'unix:///private/shim.sock'),
                (CONTAINER, owner, 'unix:///private/../shim.sock'), (CONTAINER, owner, 'unix:////private/shim.sock'),
                (CONTAINER, owner, 'unix:///private/shim.sock\n')]:
            with self.assertRaises(ValueError): subject.targets(container, candidate, address)

    def write(self, path, value):
        subject.write_private(path, value)
        return path.read_bytes()

    def retained(self, work):
        private = work/'integration'; private.mkdir(mode=0o700)
        (private/'health-bundles').mkdir(mode=0o700); (private/'aggregate-bundles').mkdir(mode=0o700)
        (work/'bin').mkdir(mode=0o700); (work/'bin/mkruntimed').write_bytes(b'synthetic committed binary')
        args, doctor, _ = self.fixture()
        self.write(work/'build.json', dict(source_commit=args[2]['revision']+'a'*33,
            binaries=dict(mkruntimed=sha((work/'bin/mkruntimed').read_bytes()))))
        self.write(work/'compiled-labels.json', dict(versions=args[12], revisions=args[13]))
        self.write(work/'storage-pressure-full-checkpoint.json', dict(owner=args[1]))
        (private/'create.stdout').write_text(CONTAINER+'\n')
        self.write(private/'engine-shim-address.json', dict(address='unix:///private/shim.sock', container_id=CONTAINER))
        request = subject.targets(CONTAINER, args[1], 'unix:///private/shim.sock')['targets'][0]['owner']
        for phase, start in [('while-full', 70), ('during-build', 500), ('while-image-ready', 1070), ('after-remove', 2070)]:
            self.write(private/('engine-'+phase+'.json'), dict(owner=request, started_monotonic_ns=start, finished_monotonic_ns=start+10))
        receipt = dict(boot_id=BOOT, invocation_id=INV, runtime_health_bundle_tested=True,
            aggregate_docker_health_tested=True, production_profile_child_build_pass=True, storage_pressure_tested=True)
        for index, phase in enumerate(subject.PHASES):
            args, doctor, bundle = self.fixture(phase == 'after-remove')
            health = doctor['health']; pair, timing = self.timing(phase, 100+index*1000)
            for prefix, value in [('doctor', args[2]), ('network-doctor', args[3]), ('combined-doctor', args[5]),
                    ('pool-doctor', args[6]), ('unified-doctor', args[7]), ('component-builds', args[8]),
                    ('operational-doctor', args[9]), ('journal-status', args[10]), ('network-health', args[11])]:
                self.write(private/(prefix+'-'+phase+'.json'), value)
            health_bundle = dict(bundle, version=2, scope=health_controls.SCOPE,
                                 snapshot=dict(components=args[8], health=health))
            data = self.write(private/'health-bundles'/(phase+'.json'), health_bundle)
            families = dict(doctor=health, endpoint=args[4], bundle=health_bundle,
                bundle_receipt=self.ack(data, 2, health_controls.SCOPE),
                bundle_identity=subject.identity(private/'health-bundles'/(phase+'.json'), data), checkpoint_timing=pair)
            for family, value in families.items():
                name = {'doctor': 'health-doctor-'+phase+'.json', 'endpoint': 'health-endpoint-'+phase+'.json',
                    'bundle': None, 'bundle_receipt': 'health-bundle-'+phase+'-receipt.json',
                    'bundle_identity': 'health-bundle-'+phase+'-identity.json', 'checkpoint_timing': 'health-checkpoint-timing-'+phase+'.json'}[family]
                raw = data if name is None else self.write(private/name, value)
                receipt.setdefault('runtime_health_'+family+'_sha256', {})[phase] = sha(raw)
            files = subject.artifact_paths(work, phase)
            data = self.write(files['bundle'], bundle)
            families = dict(bundle=bundle, doctor=doctor,
                targets=subject.targets(CONTAINER, args[1], 'unix:///private/shim.sock', phase == 'after-remove'),
                bundle_receipt=self.ack(data), bundle_identity=subject.identity(files['bundle'], data), timing=timing)
            for family, value in families.items():
                raw = data if family == 'bundle' else self.write(files[family], value)
                receipt.setdefault('aggregate_docker_health_'+family+'_sha256', {})[phase] = sha(raw)
        self.write(work/'integration-receipt.json', receipt)
        return receipt

    def test_archived_lifecycle_semantics_survive_recomputed_receipt_attacks(self):
        variants = ['valid', 'boot', 'invocation', 'engine-unqualified', 'missing', 'target-owner', 'target-mode',
                    'late', 'reordered', 'subset', 'released-target', 'bundle-inode', 'binary', 'source-label',
                    'engine-generation', 'before-engine-hold', 'after-engine-hold']
        for variant in variants:
            with self.subTest(variant=variant), tempfile.TemporaryDirectory() as directory:
                work = Path(directory); receipt = self.retained(work)
                phase = 'while-full'; files = subject.artifact_paths(work, phase)
                if variant == 'boot': receipt['boot_id'] = 'other'
                elif variant == 'invocation': receipt['invocation_id'] = '0'*32
                elif variant == 'missing': files['doctor'].unlink()
                elif variant == 'target-owner':
                    v = json.loads(files['targets'].read_text()); v['targets'][0]['owner']['generation'] = 'f'*32; files['targets'].write_bytes(encode(v))
                elif variant == 'target-mode': files['targets'].chmod(0o644)
                elif variant in ['late', 'reordered']:
                    v = json.loads(files['timing'].read_text())
                    if variant == 'late': v['deadline_monotonic_ns'] += 1
                    else: v['aggregate_started_monotonic_ns'] -= 1
                    files['timing'].write_bytes(encode(v))
                elif variant == 'subset':
                    v = json.loads(files['doctor'].read_text()); v['engines'] = []; files['doctor'].write_bytes(encode(v))
                elif variant == 'released-target':
                    phase = 'after-remove'; files = subject.artifact_paths(work, phase)
                    files['targets'].write_bytes(encode(subject.targets(CONTAINER, self.fixture()[0][1], 'unix:///private/shim.sock')))
                elif variant == 'bundle-inode':
                    v = json.loads(files['bundle_identity'].read_text()); v['inode'] += 1; files['bundle_identity'].write_bytes(encode(v))
                elif variant == 'binary': (work/'bin/mkruntimed').write_bytes(b'other binary')
                elif variant == 'source-label':
                    f = work/'build.json'; v = json.loads(f.read_text()); v['source_commit'] = 'f'*40; f.write_bytes(encode(v))
                elif variant == 'engine-generation':
                    f = work/'integration/engine-while-full.json'; v = json.loads(f.read_text()); v['owner']['generation'] = 'f'*32; f.write_bytes(encode(v))
                elif variant in ['before-engine-hold', 'after-engine-hold']:
                    f = work/('integration/engine-'+('while-full' if variant == 'before-engine-hold' else 'during-build')+'.json')
                    v = json.loads(f.read_text()); v['finished_monotonic_ns' if variant == 'before-engine-hold' else 'started_monotonic_ns'] = 101 if variant == 'before-engine-hold' else 299; f.write_bytes(encode(v))
                for family, path in files.items():
                    if path.exists(): receipt['aggregate_docker_health_'+family+'_sha256'][phase] = sha(path.read_bytes())
                (work/'integration-receipt.json').write_bytes(encode(receipt))
                # Engine approval/unfinished-RUN validation has its own complete
                # controls. Isolate it here while exercising real v8 references
                # and private artifacts, and prove its refusal still propagates.
                with patch.object(subject, 'inspect_fixture_engine_observation', return_value=dict(current_agent_and_approved_bootstrap_bound=True)) as engine:
                    if variant == 'engine-unqualified': engine.side_effect = ValueError('unqualified engine')
                    if variant == 'valid':
                        view = subject.inspect_fixture_aggregate(work, BOOT, INV)
                        self.assertTrue(view['original_five_second_health_budget_preserved'])
                        self.assertEqual(view['maximum_elapsed_ns'], 200)
                        engine.assert_called_once_with(work, BOOT, INV)
                    else:
                        with self.assertRaises((ValueError, FileNotFoundError)):
                            subject.inspect_fixture_aggregate(work, BOOT, INV)

    def test_actual_subprocess_capture_and_original_deadline_timeout(self):
        for mode in ['pass', 'timeout', 'stderr', 'oversized']:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                work = Path(directory); private = work/'integration'; private.mkdir(mode=0o700)
                (private/'aggregate-bundles').mkdir(mode=0o700); (work/'bin').mkdir(mode=0o700)
                args, doctor, bundle = self.fixture()
                self.write(private/'engine-shim-address.json', dict(address='unix:///private/shim.sock', container_id=CONTAINER))
                self.write(work/'command-fixture.json', dict(doctor=doctor, bundle=bundle, timeout=mode == 'timeout', stderr=mode == 'stderr', oversized=mode == 'oversized'))
                cli = work/'bin/mkruntimed'
                cli.write_text('''#!/usr/bin/python3
import hashlib,json,os,pathlib,sys,time
args=dict(s.split('=',1) if '=' in s else (s,'') for s in sys.argv[1:])
cfg=json.loads(pathlib.Path(args['--config']).read_text())
target=pathlib.Path(args['--engine-targets']);assert target.stat().st_mode&0o777==0o600
assert len(json.loads(target.read_text())['targets'])==1 and args['--network-socket']=='/private/network.sock'
if cfg['timeout']:
 os.write(1,b'begin\\n');os.write(2,b'waiting\\n');time.sleep(10)
if cfg['oversized']:
 os.write(1,b'x'*((1<<20)+1));sys.exit(0)
if cfg['stderr']:sys.stderr.write('synthetic private failure\\n')
if '--doctor-all' in args:
 assert args['--doctor-format']=='9';print(json.dumps(cfg['doctor']))
else:
 assert args['--diagnostic-format']=='3' and args['--diagnostic-max-files']=='3' and args['--diagnostic-max-bytes']=='1048576'
 data=json.dumps(cfg['bundle']).encode();fd=os.open(args['--diagnostic-bundle'],os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data)
 print(json.dumps(dict(version=3,scope=cfg['bundle']['scope'],collection_status='captured',observation_status='consistent',bytes=len(data),sha256=hashlib.sha256(data).hexdigest())))
'''); cli.chmod(0o500)
                start = time.monotonic_ns()
                budget = 300_000_000 if mode == 'timeout' else 5_000_000_000
                pair = dict(version=1, scope='health-doctor-and-health-bundle-checkpoint', phase='while-full',
                    configured_budget_ns=5_000_000_000, started_monotonic_ns=start, deadline_monotonic_ns=start+budget,
                    finished_monotonic_ns=start, elapsed_ns=0, report_count=2)
                call = lambda: subject.capture(work, CONTAINER, args[1], 'while-full', args, encode(doctor['health']), pair,
                    work/'command-fixture.json', '/private/network.sock')
                if mode == 'pass':
                    call()
                    timing = json.loads(subject.artifact_paths(work, 'while-full')['timing'].read_text())
                    self.assertEqual(timing['deadline_monotonic_ns'], pair['deadline_monotonic_ns'])
                    self.assertEqual(timing['report_count'], 4)
                    self.assertFalse((private/'aggregate-command-doctor-while-full-failure.json').exists())
                elif mode == 'timeout':
                    with self.assertRaises(subprocess.TimeoutExpired): call()
                    self.assertLess(time.monotonic_ns()-start, 2_000_000_000)
                    self.assertFalse(subject.artifact_paths(work, 'while-full')['timing'].exists())
                else:
                    with self.assertRaises(ValueError): call()
                    self.assertFalse(subject.artifact_paths(work, 'while-full')['timing'].exists())
                if mode != 'pass':
                    failure = json.loads((private/'aggregate-command-doctor-while-full-failure.json').read_text())
                    self.assertEqual(failure['failure'], 'command-timeout' if mode == 'timeout' else 'command-output-refused')
                    self.assertIs(failure['qualified'], False)
                    self.assertEqual(failure['stream_truncated'], dict(stdout=mode == 'oversized', stderr=False))
                    raw = private/'aggregate-command-doctor-while-full.stdout'
                    self.assertEqual(raw.stat().st_mode & 0o777, 0o600)
                    if mode == 'oversized':
                        self.assertEqual(raw.read_bytes(), b'x'*(1<<20))
                        self.assertEqual((private/'aggregate-command-doctor-while-full.stderr').read_bytes(), b'')
                    elif mode == 'timeout':
                        self.assertEqual(raw.read_bytes(), b'begin\n')
                        self.assertEqual((private/'aggregate-command-doctor-while-full.stderr').read_bytes(), b'waiting\n')

    def test_collection_limits_stop_stream_writers_before_deadline(self):
        for stream, limit in [('stdout', 1 << 20), ('stderr', 4096)]:
            with self.subTest(stream=stream):
                # A finite burst followed by a long hold distinguishes early
                # limit refusal from collecting the burst until timeout.
                code = 'import os,time;os.write(%d,b"x"*(%d));time.sleep(10)' % (1 if stream == 'stdout' else 2, limit+65536)
                start = time.monotonic_ns()
                with self.assertRaises(subject.CommandOutputLimit) as caught:
                    subject.bounded_command([sys.executable, '-c', code], start+2_000_000_000)
                self.assertLess(time.monotonic_ns()-start, 1_500_000_000)
                failure = caught.exception
                self.assertEqual(len(getattr(failure, stream)), limit+1)
                self.assertEqual(getattr(failure, 'stderr' if stream == 'stdout' else 'stdout'), b'')

    def test_collection_exact_limits_nonzero_exit_eof_and_empty_budget(self):
        code = 'import os,sys;os.write(1,b"x"*(1<<20));os.write(2,b"y"*4096);sys.exit(7)'
        result = subject.bounded_command([sys.executable, '-c', code], time.monotonic_ns()+2_000_000_000)
        self.assertEqual((result.returncode, len(result.stdout), len(result.stderr)), (7, 1 << 20, 4096))
        # Closing both pipes is not command completion or a renewed wait budget.
        start = time.monotonic_ns()
        with self.assertRaises(subprocess.TimeoutExpired) as caught:
            subject.bounded_command([sys.executable, '-c', 'import os,time;os.close(1);os.close(2);time.sleep(10)'], start+150_000_000)
        self.assertLess(time.monotonic_ns()-start, 1_000_000_000)
        self.assertEqual((caught.exception.stdout, caught.exception.stderr), (b'', b''))
        with patch.object(subject.subprocess, 'Popen') as spawn, self.assertRaises(subprocess.TimeoutExpired):
            subject.bounded_command(['/never-execute'], time.monotonic_ns()-1)
        spawn.assert_not_called()

    def test_auditor_requires_pressure_before_stage_access(self):
        spec = importlib.util.spec_from_file_location('aggregate_live_audit', Path(__file__).with_name('audit-runtime-docker-trial.py'))
        auditor = importlib.util.module_from_spec(spec); spec.loader.exec_module(auditor)
        for flag in ['require_engine_observation', 'require_aggregate_docker_health']:
            args = SimpleNamespace(work=Path('/not-a-trial'), scenario='concurrent', **{flag: True})
            with patch.object(auditor, 'validate_stage') as stage, self.assertRaisesRegex(ValueError, '^engine observation requires pressure scenario$'):
                auditor.audit(args)
            stage.assert_not_called()


if __name__ == '__main__':
    unittest.main()
