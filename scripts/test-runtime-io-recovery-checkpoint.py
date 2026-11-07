#!/usr/bin/env python3
import importlib.util,json,tempfile,unittest
from pathlib import Path
from runtime_io_recovery_checkpoint import sync_sequence,payload_sha256,IO_BYTES
from runtime_recovery_checkpoint import inspect_fixture_recovery
spec=importlib.util.spec_from_file_location('recovery_controls',Path(__file__).with_name('test-runtime-recovery-checkpoint.py'))
c=importlib.util.module_from_spec(spec);spec.loader.exec_module(c)
BOOT=c.BOOT;INV=c.INV;raw=c.raw;sha=c.sha


class SyncedIOTests(unittest.TestCase):
    def fixture(self,work,mode=None):
        c.RecoveryTests().retained_active(work);private=work/'integration'
        r=json.loads((work/'recovery-checkpoint.json').read_bytes());life=json.loads((work/'integration-receipt.json').read_bytes())
        r['schema_version']=4;life['runtime_io_during_crash_tested']=True
        def write(parent,name,value):
            data=value if type(value) is bytes else raw(value);p=parent/name;p.write_bytes(data);p.chmod(0o600);return sha(data)
        beginning=b'#9 0.01 G8_OVERLAP_RUN_BEGIN\n'
        expected=payload_sha256(6)
        progress=lambda n:('#9 '+str(n)+'.0 G8_IO_SYNCED_SEQ='+str(n)+'\n').encode()
        prefixes={n:beginning+b''.join(progress(i) for i in range(1,n+1)) for n in range(1,7)}
        err=prefixes[6]+b'#9 45.0 G8_IO_FINAL_SEQ=6\n#9 45.0 G8_IO_FINAL_SHA256='+expected.encode()+b'\n#9 45.0 G8_OVERLAP_RUN_END\nCACHED\nexit code: 37\n'
        if mode=='final-hash':err=err.replace(expected.encode(),b'0'*64)
        if mode=='final-count':err=err.replace(b'G8_IO_FINAL_SEQ=6',b'G8_IO_FINAL_SEQ=3')
        out=(private/'workload.stdout').read_bytes()+b'G8_IO_IMAGE_SHA256='+expected.encode()+b'\nG8_IO_IMAGE_BYTES=4194304\nG8_IO_IMAGE_HEADER=G8_IO_DATA_SEQ=0000006\n'
        if mode=='image-hash':out=out.replace(expected.encode(),b'0'*64)
        if mode=='stale-sequence':out=out.replace(b'G8_IO_DATA_SEQ=0000006',b'G8_IO_DATA_SEQ=0000005')
        if mode=='stale-payload':out=out.replace(expected.encode(),payload_sha256(5).encode());err=err.replace(expected.encode(),payload_sha256(5).encode())
        if mode=='image-size':out=out.replace(b'G8_IO_IMAGE_BYTES=4194304',b'G8_IO_IMAGE_BYTES=4194303')
        workload=json.loads((work/'workload-checkpoint.json').read_bytes())
        workload.update(workload_stdout_sha256=write(private,'workload.stdout',out),workload_stderr_sha256=write(private,'workload.stderr',err))
        life['workload_checkpoint_sha256']=write(work,'workload-checkpoint.json',workload)
        frames={}
        for phase,n,t,pid in [('before-kill',1,100,r['runtime_pid_before']),('confirmed-dead',2,175,r['runtime_pid_before']),('after-kill',3,200,r['runtime_pid_before']),('adoption-baseline',4,275,r['runtime_pid_after']),('after-adoption',5,300,r['runtime_pid_after'])]:
            if mode=='dead-no-progress' and phase=='after-kill':n=2
            if mode=='adopted-no-progress' and phase=='after-adoption':n=4
            if mode=='adoption-uses-old-pid' and phase=='adoption-baseline':pid=r['runtime_pid_before']
            if mode=='adoption-clock' and phase=='adoption-baseline':t=350
            name='active-run-'+phase+'.stderr'
            frames[phase]=dict(observed_monotonic_ns=t,runtime_pid=pid,markers=['BEGIN'],stderr_snapshot=name,stderr_sha256=write(private,name,prefixes[n]),io_synced_seq=n)
            if phase=='confirmed-dead':frames[phase]['runtime_exit_status']=0 if mode=='not-dead' else -9
        if mode=='missing-dead':frames.pop('confirmed-dead')
        if mode=='missing-adoption-baseline':frames.pop('adoption-baseline')
        if mode=='dead-clock':frames['confirmed-dead']['observed_monotonic_ns']=145
        if mode=='bool-sequence':frames['before-kill']['io_synced_seq']=True
        active=dict(version=3,boot_id=BOOT,invocation_id=INV,stages=frames,cleanup_qualified=False)
        if mode=='legacy-four-frame':active['version']=2;frames.pop('adoption-baseline')
        r['active_run_checkpoint_sha256']=write(work,'active-run-checkpoint.json',active)
        owner=r['before']['ownership'][0]
        lease=dict(sandbox_id=owner['sandbox_id'],sandbox_generation=owner['generation'],peer_cid=owner['child_cid'],state='RELEASED',offline_check='e2fsck-clean-sha256:'+'a'*64,counters=dict(written_bytes=6*IO_BYTES))
        if mode=='native-underreports':lease['counters']['written_bytes']-=1
        if mode=='native-bool':lease['counters']['written_bytes']=True
        if mode=='native-owner':lease['sandbox_generation']='0'*32
        if mode=='native-offline':lease['offline_check']=''
        if mode=='native-active':lease['state']='ACTIVE'
        (private/'storage-state').mkdir(mode=0o700)
        write(private/'storage-state','state.json',dict(exports={'export':lease}))
        if mode=='schema':r['schema_version']=3
        if mode=='masked-io':life['runtime_io_during_crash_tested']=False
        life['recovery_checkpoint_sha256']=write(work,'recovery-checkpoint.json',r);write(work,'integration-receipt.json',life)

    def test_synced_progress_is_executed_canonical_increasing_and_bounded(self):
        self.assertEqual(sync_sequence(b'#9 0.1 G8_IO_SYNCED_SEQ=1\n#9 1.0 G8_IO_SYNCED_SEQ=4\n'),4)
        for bad in [b'#9 RUN echo G8_IO_SYNCED_SEQ=1\n',b'#9 0.1 G8_IO_SYNCED_SEQ=01\n',b'#9 0.1 G8_IO_SYNCED_SEQ=0\n',b'#9 0.1 G8_IO_SYNCED_SEQ=1048577\n',b'#9 0.1 G8_IO_SYNCED_SEQ=2\n#9 0.2 G8_IO_SYNCED_SEQ=2\n',b'x'*131073]:
            with self.subTest(stream=bad[:80]),self.assertRaises(ValueError):sync_sequence(bad)

    def test_synced_outage_adoption_data_readback_and_native_floor_qualify_only_named_scope(self):
        with tempfile.TemporaryDirectory() as d:
            work=Path(d);self.fixture(work);view=inspect_fixture_recovery(work,BOOT,INV,'runtime-crash-during-io')
            self.assertIs(view['synced_write_outage_observed'],True);self.assertIs(view['built_image_data_readback_observed'],True);self.assertIs(view['native_write_coverage_observed'],True);self.assertIs(view['cleanup_qualified'],False)
            for scenario in ['runtime-restart','runtime-crash','runtime-crash-during-build']:
                with self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV,scenario)

    def test_recomputed_receipts_cannot_mask_missing_outage_progress_data_drift_or_native_underreporting(self):
        for mode in ['dead-no-progress','adopted-no-progress','missing-adoption-baseline','legacy-four-frame','adoption-uses-old-pid','adoption-clock','final-hash','final-count','image-hash','image-size','stale-sequence','stale-payload','missing-dead','not-dead','dead-clock','bool-sequence','native-underreports','native-bool','native-owner','native-offline','native-active','schema','masked-io']:
            with tempfile.TemporaryDirectory() as d:
                work=Path(d);self.fixture(work,mode)
                with self.subTest(mode=mode),self.assertRaises(ValueError):inspect_fixture_recovery(work,BOOT,INV,'runtime-crash-during-io')


if __name__=='__main__':unittest.main()
