#!/usr/bin/env python3
import copy,hashlib,json,tempfile,unittest
from pathlib import Path
from runtime_concurrency_checkpoint import inspect_fixture_concurrency
from runtime_docker_concurrency import sandbox_id_for_container
from runtime_primary_docker import fingerprint
BOOT='509d5e93-31b8-4e8f-925a-4caa9fcf4788';INV='9'*32
raw=lambda v:json.dumps(v).encode()
sha=lambda b:hashlib.sha256(b).hexdigest()


class ConcurrentCheckpointTests(unittest.TestCase):
    def fixture(self,work,mode=None):
        p=work/'integration';p.mkdir(mode=0o700)
        def write(parent,name,v):
            data=v if type(v) is bytes else raw(v);f=parent/name;f.write_bytes(data);f.chmod(0o600);return sha(data)
        before={case:dict(child_boot_id=str(i)*8+'-'+str(i)*4+'-4'+str(i)*3+'-8'+str(i)*3+'-'+str(i)*12,engine_id='child-'+case,canary=case*32,sentinel_image_id='sha256:'+case*64) for case,i in [('a',1),('b',2)]}
        after={case:dict(v,saved_image_sha256='c'*64 if case=='a' else 'd'*64,built_image_id='sha256:'+('e' if case=='a' else 'f')*64) for case,v in before.items()}
        owners=[dict(sandbox_id=sandbox_id_for_container(case*64),generation=case*32,state='RUNNING',cpus=list(range(8 if case=='a' else 12,12 if case=='a' else 16)),memory_bytes=3<<30,child_cid=cid,agent_port=7200) for case,cid in [('a',40),('b',41)]]
        storage=[dict(sandbox_id=o['sandbox_id'],sandbox_generation=o['generation'],export_generation=('c' if i==0 else 'd')*32,peer_cid=o['child_cid'],port=10809+i,filesystem_uuid=str(i+3)*8+'-'+str(i+3)*4+'-4'+str(i+3)*3+'-8'+str(i+3)*3+'-'+str(i+3)*12,image_identity=dict(device=123,inode=100+i),image_path_sha256=('e' if i==0 else 'f')*64) for i,o in enumerate(owners)]
        rebuilt=dict(after['a'],saved_image_sha256='1'*64,built_image_id='sha256:'+'2'*64)
        r=dict(schema_version=2,boot_id=BOOT,invocation_id=INV,before=before,after=after,survivor=copy.deepcopy(after['a']),two_running_owners=owners,case_owners=dict(zip(['a','b'],copy.deepcopy(owners))),remaining_owner=copy.deepcopy(owners[0]),two_storage_exports=storage,survivor_storage_export=[copy.deepcopy(storage[0])],run_overlap_observed=True,sibling_removed_without_survivor_change=True,cleanup_qualified=False,survivor_rebuilt=rebuilt,original_archive_after_rebuild_sha256=after['a']['saved_image_sha256'])
        if mode=='shared-engine':r['before']['b']['engine_id']=r['before']['a']['engine_id'];r['after']['b']['engine_id']=r['before']['a']['engine_id']
        if mode=='shared-canary':r['before']['b']['canary']=r['before']['a']['canary'];r['after']['b']['canary']=r['before']['a']['canary']
        if mode=='cpu-overlap':owners[1]['cpus']=owners[0]['cpus'][:]
        if mode=='cpu-float':owners[0]['cpus'][0]=8.0
        if mode=='backing-overlap':storage[1]['image_identity']=storage[0]['image_identity'].copy()
        if mode=='survivor-drift':r['survivor']['engine_id']='replacement-engine'
        if mode=='original-archive':r['original_archive_after_rebuild_sha256']='0'*64
        if mode=='remaining-owner':r['remaining_owner']=copy.deepcopy(owners[1])
        if mode=='wrong-epoch':r['boot_id']=before['a']['child_boot_id']
        if mode=='legacy':r['schema_version']=1
        if mode=='cleanup-mask':r['cleanup_qualified']=True
        if mode=='storage-owner':storage[1]['sandbox_generation']='0'*32
        (p/'manifests').mkdir(mode=0o700);write(p/'manifests','gce-mk2.json',dict(kernel_release='test-kernel'))
        write(p,'two-child-ownership.stdout',dict(source='durable-lifecycle',version=1.0 if mode=='owner-version-float' else 1,sandboxes=owners))
        for case in ['a','b']:
            phases=[('before',r['before'][case],False),('after',r['after'][case],True)]
            if case=='a':phases.extend([('survivor',r['survivor'],True),('survivor-rebuilt',r['survivor_rebuilt'],True)])
            for phase,view,built in phases:
                prefix=phase+'-'+case
                for label,key in [('boot','child_boot_id'),('canary','canary'),('own-image','sentinel_image_id')]:write(p,prefix+'-'+label+'.stdout',(view[key]+'\n').encode())
                write(p,prefix+'-engine.stdout',dict(ID=view['engine_id'],KernelVersion='test-kernel',ServerVersion='29.9.0',SecurityOptions=['name=rootless']))
                if built:write(p,prefix+'-saved-image.stdout',(view['saved_image_sha256']+' /home/rootless/.local/tmp/g8-build/image.tar\n').encode())
        write(p,'survivor-original-archive.stdout',(r['original_archive_after_rebuild_sha256']+' original.tar\n').encode())
        write(p,'two-child-storage.json',storage);write(p,'survivor-storage.json',r['survivor_storage_export'])
        (p/'storage-state').mkdir(mode=0o700)
        released={str(i):dict(v,state='RELEASED',offline_check='e2fsck-clean-sha256:'+'1'*64) for i,v in enumerate(storage)}
        if mode=='released-owner':released['1']['sandbox_generation']='0'*32
        if mode=='released-generation':released['1']['export_generation']='0'*32
        write(p/'storage-state','state.json',dict(exports=released))
        write(p,'survivor-ownership.stdout',dict(sandboxes=[r['remaining_owner']]))
        for case in ['a','b']:
            item=dict(Id=case*64,Config=dict(User='1000:1000'),HostConfig=dict(Runtime='io.containerd.multikernel.v2',Privileged=False,CapDrop=['ALL'],SecurityOpt=['no-new-privileges=true']),Mounts=[dict(Destination='/work/input',RW=False)])
            if mode=='outer-authority' and case=='b':item['HostConfig']['Privileged']=True
            if mode=='mount-numeric-mask' and case=='b':item['Mounts'][0]['RW']=0
            write(p,'owner-map-'+case+'.stdout',[item])
            for phase in ['before','after','survivor','survivor-rebuilt'] if case=='a' else ['before','after']:
                write(p,phase+'-'+case+'-foreign-image.stderr',('No such image: g8-isolation:'+('b' if case=='a' else 'a')+'\n').encode())
                write(p,phase+'-'+case+'-foreign-image.exit.json',dict(returncode=0 if mode=='foreign-accepted' and case=='b' else 1))
        markers=['G8_MULTISTAGE_BUILD_PASS','G8_REPEAT_BUILD_PASS','G8_BUILT_IMAGE_RUN_PASS','G8_FAILED_BUILD_REJECTED','G8_IMAGE_SAVE_EXIT=0','G8_BUILD_RESOURCES_REMOVED','G8_BUILD_WORKLOAD_PASS']
        def stdout(image):return ('\n'.join(markers)+'\nG8_BUILD_IMAGE_ID='+image+'\nCapEff:\t0000000000000000\nCapBnd:\t0000000000000000\nG8_NESTED_AUTHORITY_BEGIN\nNoNewPrivs:\t1\nSeccomp:\t2\n0 1000 1\nG8_NESTED_AUTHORITY_END\n').encode()
        prefix=b'#9 0.1 G8_OVERLAP_RUN_BEGIN\n';stderr=prefix+b'#9 45.1 G8_OVERLAP_RUN_END\nCACHED\nexit code: 37\n'
        r['case_stdout_sha256']={};r['case_stderr_sha256']={}
        for case in ['a','b']:
            out=stdout(after[case]['built_image_id'])+after[case]['saved_image_sha256'].encode()+b'\nG8_CONCURRENT_PROBE_EXIT=0\nG8_CONCURRENT_BUILD_READY\nG8_CONCURRENT_RELEASED\n'
            if mode=='missing-build' and case=='b':out=out.replace(b'G8_REPEAT_BUILD_PASS\n',b'')
            r['case_stdout_sha256'][case]=write(p,case+'-workload.stdout',out);r['case_stderr_sha256'][case]=write(p,case+'-workload.stderr',stderr)
        r['survivor_rebuild_stdout_sha256']=write(p,'survivor-rebuild.stdout',stdout(rebuilt['built_image_id']))
        r['survivor_rebuild_stderr_sha256']=write(p,'survivor-rebuild.stderr',b'CACHED\nexit code: 37\n')
        baseline=dict(schema_version=2,boot_id=BOOT,engine_id_sha256=fingerprint('primary'),inventories={k:dict(count=0,sha256='0'*64) for k in ['images','containers','volumes','networks']})
        write(p,'primary-docker-before.json',baseline)
        phases={'overlap':None,'two-built-a':after['a']['built_image_id'],'two-built-b':after['b']['built_image_id'],'survivor':after['a']['built_image_id'],'survivor-rebuild':rebuilt['built_image_id']}
        r['primary_snapshots_sha256']={}
        for phase,image in phases.items():
            sample=dict(baseline,checked_absent_image=image,child_image_absent=image is not None)
            if mode=='primary-image' and phase=='two-built-b':sample['child_image_absent']=False
            r['primary_snapshots_sha256'][phase]=write(p,'concurrency-primary-'+phase+'.json',sample)
        captures={phase:{} for phase in ['before','after']}
        for phase in captures:
            for case in ['a','b']:
                data=prefix
                if mode=='finished-overlap' and phase=='after' and case=='b':data+=b'#9 45.1 G8_OVERLAP_RUN_END\n'
                if mode=='unbound-overlap' and phase=='after' and case=='b':data+=b'synthetic forged snapshot\n'
                captures[phase][case]=write(p,'run-overlap-'+phase+'-'+case+'.stderr',data)
        overlap=dict(version=1,boot_id=BOOT,invocation_id=INV,before_monotonic_ns=100,after_monotonic_ns=200,captures=captures,primary_sha256=r['primary_snapshots_sha256']['overlap'],cleanup_qualified=False)
        if mode=='overlap-clock':overlap['after_monotonic_ns']=99
        if mode=='overlap-budget':overlap['after_monotonic_ns']=10_000_000_101
        r['run_overlap_checkpoint_sha256']=write(p,'run-overlap.json',overlap)
        receipt=dict(boot_id=BOOT,invocation_id=INV,concurrent_guest_builds_tested=True,production_profile_child_build_pass=True,storage_pressure_tested=False,runtime_restart_tested=False,runtime_crash_tested=False,runtime_crash_during_build_tested=False,runtime_io_during_crash_tested=False)
        if mode=='wrong-scenario':receipt['concurrent_guest_builds_tested']=False
        receipt['concurrency_checkpoint_sha256']=write(p,'concurrency-checkpoint.json',r);write(work,'integration-receipt.json',receipt)

    def test_two_private_builds_overlap_survivor_rebuild_and_original_archive_preservation(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.fixture(work);view=inspect_fixture_concurrency(work,BOOT,INV)
            self.assertIs(view['two_child_engines_and_storage_separate'],True);self.assertIs(view['survivor_rebuild_and_original_archive_preservation'],True);self.assertIs(view['cleanup_qualified'],False)

    def test_recomputed_receipts_cannot_mask_shared_identity_overlap_drift_or_primary_leak(self):
        for mode in ['shared-engine','shared-canary','cpu-overlap','cpu-float','backing-overlap','storage-owner','released-owner','released-generation','survivor-drift','original-archive','remaining-owner','wrong-epoch','legacy','cleanup-mask','outer-authority','owner-version-float','mount-numeric-mask','foreign-accepted','missing-build','primary-image','finished-overlap','unbound-overlap','overlap-clock','overlap-budget','wrong-scenario']:
            with tempfile.TemporaryDirectory() as d:
                work=Path(d);self.fixture(work,mode)
                with self.subTest(mode=mode),self.assertRaises(ValueError):inspect_fixture_concurrency(work,BOOT,INV)

    def test_caller_epoch_and_ambiguous_or_unbounded_private_artifacts_refuse(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.fixture(work)
            for boot,inv in [('11111111-1111-4111-8111-111111111111',INV),(BOOT,'8'*32)]:
                with self.assertRaises(ValueError):inspect_fixture_concurrency(work,boot,inv)
            p=work/'integration/concurrency-checkpoint.json';original=p.read_bytes()
            for bad in [b'{"schema_version":2,'+original[1:],b' '*(1<<20)+original]:
                p.write_bytes(bad)
                with self.assertRaises(ValueError):inspect_fixture_concurrency(work,BOOT,INV)
            p.write_bytes(original)
            self.assertTrue(inspect_fixture_concurrency(work,BOOT,INV)['simultaneous_unfinished_runs_observed'])


if __name__=='__main__':unittest.main()
