#!/usr/bin/env python3
import copy
import ast
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest
import tempfile
import subprocess
import time
from runtime_bounded_command import bounded_command, CommandOutputLimit
from runtime_health_bundle_checkpoint import inspect_fixture_health
from runtime_health_bundle_checkpoint import qualify, qualify_bundle, qualify_timing, SCOPE, UNOBSERVED
from runtime_diagnostic_bundle_checkpoint import EXCLUDED
spec=importlib.util.spec_from_file_location('op_controls',Path(__file__).with_name('test-runtime-operational-doctor-checkpoint.py'))
controls=importlib.util.module_from_spec(spec);spec.loader.exec_module(controls)

class HealthCheckpointTests(unittest.TestCase):
    def live_command(self, name, work, deadline):
        # Execute the actual command expression from the live harness without
        # importing that harness's privileged trial setup or lifecycle actions.
        tree=ast.parse(Path(__file__).with_name('test-runtime-docker-profile-live.py').read_text())
        values=[n.value for n in ast.walk(tree) if isinstance(n,ast.Assign) and any(isinstance(t,ast.Name) and t.id==name for t in n.targets)]
        self.assertEqual(len(values),1)
        return eval(compile(ast.Expression(values[0]),'<live-health-command>','eval'),dict(
            work=work,host_config=work/'config.json',network_socket=work/'network.sock',
            destination=work/'bundle.json',native_record_fault=False,
            bounded_command=bounded_command,subprocess=subprocess,
            remaining=max(0,(deadline-time.monotonic_ns())/1e9),health_deadline_ns=deadline))
    def test_live_commands_refuse_stream_flood_before_writer_completes(self):
        for name in ['health_doctor','health_bundle']:
            for fd in [1,2]:
                with self.subTest(command=name,stream=fd),tempfile.TemporaryDirectory() as directory:
                    work=Path(directory);(work/'bin').mkdir(mode=0o700)
                    marker=work/'burst-completed';cli=work/'bin/mkruntimed'
                    cli.write_text('#!/usr/bin/python3\nimport os,pathlib,time\nos.write('+str(fd)+',b"x"*(8<<20))\npathlib.Path('+repr(str(marker))+').touch()\ntime.sleep(5)\n');cli.chmod(0o500)
                    with self.assertRaises(CommandOutputLimit) as caught:
                        self.live_command(name,work,time.monotonic_ns()+2_000_000_000)
                    self.assertFalse(marker.exists())
                    self.assertEqual(len(caught.exception.stdout if fd==1 else caught.exception.stderr),(1<<20 if fd==1 else 4096)+1)
    def test_live_command_normal_status_and_shared_deadline_survive_eof(self):
        for name in ['health_doctor','health_bundle']:
            with self.subTest(command=name),tempfile.TemporaryDirectory() as directory:
                work=Path(directory);(work/'bin').mkdir(mode=0o700);cli=work/'bin/mkruntimed'
                cli.write_text('#!/usr/bin/python3\nimport os,sys\nos.write(1,b"synthetic health");os.write(2,b"synthetic refusal");sys.exit(7)\n');cli.chmod(0o500)
                result=self.live_command(name,work,time.monotonic_ns()+1_000_000_000)
                self.assertEqual((result.returncode,result.stdout,result.stderr),(7,b'synthetic health',b'synthetic refusal'))
                cli.chmod(0o600)
                cli.write_text('#!/usr/bin/python3\nimport os,time\nos.close(1);os.close(2);time.sleep(5)\n');cli.chmod(0o500)
                start=time.monotonic_ns()
                with self.assertRaises(subprocess.TimeoutExpired):self.live_command(name,work,start+200_000_000)
                self.assertLess(time.monotonic_ns()-start,1_000_000_000)
    def fixture(self,released=False):
        s,e,n,c,p,u,b,versions,revisions,op=controls.OperationalCheckpointTests().fixture(released)
        journal=dict(version=1,source='in-process-lifecycle-journal',status='idle',sequence=s['ownership']['sequence'],pending_completions=0,journal_failed=False)
        network=dict(version=1,source='host-network-backend-check',primary_boot_id=controls.BOOT,status='idle' if released else 'checked',endpoints=[dict(endpoint_generation=v['endpoint_generation'],descriptor_sha256=v['descriptor_sha256'],code='CHECK_PASSED') for v in n['network']['endpoints']])
        health=dict(version=8,scope='operational-journal-and-recorded-network-health-observations',status='consistent',revision=s['revision'],boot_id=controls.BOOT,boot_stable=True,operational=op,journal=journal,network_health=network,findings=[],admission_guaranteed=False,unobserved=UNOBSERVED)
        args=[controls.BOOT,controls.OWNER,s,n,e,c,p,u,b,op,journal,network,versions,revisions]
        bundle=dict(version=2,scope=SCOPE,collection_status='captured',started_at='2026-11-06T00:00:00Z',finished_at='2026-11-06T00:00:01Z',snapshot=dict(components=b,health=health),excluded=EXCLUDED)
        return args,health,bundle
    def capture(self,args,health,bundle, released=False):
        data=json.dumps(bundle).encode();receipt=dict(version=2,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(data),sha256=hashlib.sha256(data).hexdigest())
        return qualify_bundle(data,json.dumps(receipt).encode(),json.dumps(health).encode(),*args,released=released)
    def test_live_and_released_exact_independent_references(self):
        for released in [False,True]:
            a,h,b=self.fixture(released);self.assertEqual(self.capture(a,h,b,released),b)
    def test_forged_health_and_bundle_cannot_hide_behind_matching_receipt(self):
        for mode in ['boot','sequence','health-failed','journal-pending','version-float','admission','unobserved','private','components','extra','legacy']:
            a,h,b=copy.deepcopy(self.fixture())
            if mode=='boot':h['boot_id']='other'
            elif mode=='sequence':h['journal']['sequence']+=1
            elif mode=='health-failed':h['network_health']['status']='degraded'
            elif mode=='journal-pending':h['journal']['status']='pending';h['journal']['pending_completions']=1
            elif mode=='version-float':h['version']=8.0
            elif mode=='admission':h['admission_guaranteed']=True
            elif mode=='unobserved':h['unobserved']=[]
            elif mode=='private':h['private-token']='synthetic-private-canary'
            elif mode=='components':b['snapshot']['components']['artifact_verified']=True
            elif mode=='extra':b['extra']='synthetic-private-canary'
            elif mode=='legacy':b['version']=1
            with self.subTest(mode=mode),self.assertRaises(ValueError):self.capture(a,h,b)
    def test_independently_forged_references_duplicates_and_bounds_refuse(self):
        for index in [2,3,5,6,7,8,9,10,11]:
            a,h,b=copy.deepcopy(self.fixture());a[index]['status']='inconclusive'
            with self.subTest(index=index),self.assertRaises(ValueError):self.capture(a,h,b)
        a,h,b=self.fixture();raw=json.dumps(h).encode()
        for data in [b'{"version":8,'+raw[1:],b' '*(8<<20)+raw]:
            with self.assertRaises(ValueError):qualify(data,*a)
        data=json.dumps(b).encode();receipt=dict(version=2,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(data),sha256='0'*64)
        with self.assertRaises(ValueError):qualify_bundle(data,json.dumps(receipt).encode(),raw,*a)
    def retained(self, work):
        private=work/'integration';private.mkdir(mode=0o700);(private/'health-bundles').mkdir(mode=0o700)
        receipt=dict(boot_id=controls.BOOT,invocation_id='9'*32,runtime_health_bundle_tested=True,production_profile_child_build_pass=True,storage_pressure_tested=True)
        def write(parent,name,value):
            raw=json.dumps(value).encode();f=parent/name;f.write_bytes(raw);f.chmod(0o600);return raw
        write(work,'storage-pressure-full-checkpoint.json',dict(owner=controls.OWNER))
        for index,phase in enumerate(['while-full','while-image-ready','after-remove']):
            a,h,b=self.fixture(phase=='after-remove')
            for prefix,value in [('doctor',a[2]),('network-doctor',a[3]),('combined-doctor',a[5]),('pool-doctor',a[6]),('unified-doctor',a[7]),('component-builds',a[8]),('operational-doctor',a[9]),('journal-status',a[10]),('network-health',a[11])]:write(private,prefix+'-'+phase+'.json',value)
            write(work,'compiled-labels.json',dict(versions=a[12],revisions=a[13]))
            raw=write(private/'health-bundles',phase+'.json',b);st=(private/'health-bundles'/(phase+'.json')).stat()
            identity=dict(device=st.st_dev,inode=st.st_ino,uid=st.st_uid,links=st.st_nlink,mode='0600',bytes=st.st_size,sha256=hashlib.sha256(raw).hexdigest())
            ack=dict(version=2,scope=SCOPE,collection_status='captured',observation_status='consistent',bytes=len(raw),sha256=identity['sha256'])
            start=100+index*1000
            timing=dict(version=1,scope='health-doctor-and-health-bundle-checkpoint',phase=phase,configured_budget_ns=5_000_000_000,started_monotonic_ns=start,deadline_monotonic_ns=start+500,finished_monotonic_ns=start+100,elapsed_ns=100,report_count=2)
            for family,parent,name,value in [('doctor',private,'health-doctor-'+phase+'.json',h),('endpoint',private,'health-endpoint-'+phase+'.json',a[4]),('bundle',private/'health-bundles',phase+'.json',b),('bundle_receipt',private,'health-bundle-'+phase+'-receipt.json',ack),('bundle_identity',private,'health-bundle-'+phase+'-identity.json',identity),('checkpoint_timing',private,'health-checkpoint-timing-'+phase+'.json',timing)]:
                receipt.setdefault('runtime_health_'+family+'_sha256',{})[phase]=hashlib.sha256(write(parent,name,value)).hexdigest()
        write(work,'integration-receipt.json',receipt)
        return receipt
    def test_lifecycle_requalifies_private_endpoint_and_all_three_products(self):
        with tempfile.TemporaryDirectory() as directory:
            work=Path(directory);receipt=self.retained(work)
            self.assertEqual(inspect_fixture_health(work,controls.BOOT,'9'*32)['maximum_elapsed_ns'],100)
            with self.assertRaises(ValueError):inspect_fixture_health(work,controls.BOOT,'8'*32)
            f=work/'integration/health-endpoint-while-full.json';data=json.loads(f.read_bytes());data['generation']='f'*32;f.write_text(json.dumps(data))
            # A recomputed endpoint receipt hash cannot mask a forged reference.
            receipt['runtime_health_endpoint_sha256']['while-full']=hashlib.sha256(f.read_bytes()).hexdigest();(work/'integration-receipt.json').write_text(json.dumps(receipt))
            with self.assertRaises(ValueError):inspect_fixture_health(work,controls.BOOT,'9'*32)
    def test_lifecycle_missing_frame_tamper_and_identity_refuse(self):
        for kind in ['missing','hash','identity']:
            with tempfile.TemporaryDirectory() as directory:
                work=Path(directory);receipt=self.retained(work)
                if kind=='missing':(work/'integration/health-checkpoint-timing-after-remove.json').unlink()
                elif kind=='hash':(work/'integration/health-doctor-while-full.json').write_bytes(b'{}')
                else:
                    f=work/'integration/health-bundle-while-full-identity.json';v=json.loads(f.read_text());v['inode']+=1;f.write_text(json.dumps(v))
                    receipt['runtime_health_bundle_identity_sha256']['while-full']=hashlib.sha256(f.read_bytes()).hexdigest();(work/'integration-receipt.json').write_text(json.dumps(receipt))
                with self.subTest(kind=kind),self.assertRaises((ValueError,FileNotFoundError)):inspect_fixture_health(work,controls.BOOT,'9'*32)
    def test_health_pair_budget_is_separate_from_legacy_sequence(self):
        t=dict(version=1,scope='health-doctor-and-health-bundle-checkpoint',phase='while-full',configured_budget_ns=5_000_000_000,started_monotonic_ns=100,deadline_monotonic_ns=200,finished_monotonic_ns=150,elapsed_ns=50,report_count=2)
        self.assertEqual(qualify_timing(json.dumps(t).encode(),'while-full'),t)
        for key,value in [('report_count',8),('report_count',2.0),('elapsed_ns',51),('finished_monotonic_ns',201),('scope','doctor-and-diagnostic-bundle-checkpoint')]:
            bad=dict(t,**{key:value})
            with self.assertRaises(ValueError):qualify_timing(json.dumps(bad).encode(),'while-full')

if __name__=='__main__':unittest.main()
