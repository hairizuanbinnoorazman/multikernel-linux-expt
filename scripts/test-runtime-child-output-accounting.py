#!/usr/bin/env python3
"""Recomputed counter/receipt attacks against held-child output acceptance."""
import copy,hashlib,importlib.util,json,os,subprocess,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_trial_agent_profile as agent_profile
import runtime_child_output_accounting as output
from runtime_docker_concurrency import sandbox_id_for_container

spec=importlib.util.spec_from_file_location('logging_fixture',Path(__file__).with_name('test-runtime-child-log-checkpoint.py'))
fixture=importlib.util.module_from_spec(spec);spec.loader.exec_module(fixture)
BOOT,INV=fixture.BOOT,fixture.INV
sha=lambda b:hashlib.sha256(b).hexdigest()
encoded=lambda value:json.dumps(value,sort_keys=True).encode()


def stream(n):
    return dict(observed_bytes=n,acknowledged_bytes=n,buffered_bytes=0,pending_bytes=0,dropped_bytes=0,buffered_peak_bytes=n,limit_bytes=4<<20,truncated=False,accounting_exhausted=False)

def disposition(n):
    return dict(written_bytes=n,no_destination_bytes=0,pressure_dropped_bytes=0,unknown_prefix_bytes=0,in_flight_bytes=0,replay_exposure_bytes=0,exposure_exhausted=False)

def frames(container):
    records=[dict(sequence=i,id=('%064x'%i),pid=100+i,status=3,exit=0,stdout_offset=0,stderr_offset=0) for i in range(2,18)]
    before=dict(version=1,scope='private-shim-completed-output',status='observed',code='COMPLETED_OUTPUT_OBSERVED',sandbox_id=sandbox_id_for_container(container),generation='a'*32,task_id=container,namespace='moby',bundle_identity=dict(device=1,inode=2,uid=os.geteuid()),tail=dict(version=1,deleted_count=17,evicted_count=1,accounting_exhausted=False,records=records))
    after=copy.deepcopy(before)
    final=dict(sequence=18,id='e'*64,pid=901,status=3,exit=37,stdout_offset=output.STDOUT_BYTES,stderr_offset=len(output.STDERR),final_output_accounting=dict(state='validated',report=dict(version=1,id='e'*64,pid=901,status='STOPPED',stdout=stream(output.STDOUT_BYTES),stderr=stream(len(output.STDERR)))),output_disposition=dict(version=1,stdout=disposition(output.STDOUT_BYTES),stderr=disposition(len(output.STDERR))))
    after['tail']=dict(version=1,deleted_count=18,evicted_count=2,accounting_exhausted=False,records=copy.deepcopy(records[1:])+[final])
    return before,after


class OutputTests(unittest.TestCase):
    def qualify(self,before,after,container='b'*64,stdout=None,stderr=None,status=37):
        return output.qualify_pair(encoded(before),encoded(after),container,b'\0'*output.STDOUT_BYTES if stdout is None else stdout,output.STDERR if stderr is None else stderr,status)

    def test_exact_probe_and_bounded_tail_eviction_accept(self):
        before,after=frames('b'*64);view=self.qualify(before,after)
        self.assertEqual(view['stdout_received_bytes'],8192);self.assertEqual(view['stderr_received_bytes'],17)
        self.assertFalse(view['docker_log_retention_qualified']);self.assertFalse(view['full_g7_g8_qualified'])
        before.pop('tail');before['status']='unobserved';before['code']='COMPLETED_OUTPUT_NOT_OBSERVED'
        after['tail']['deleted_count']=1;after['tail']['evicted_count']=0;after['tail']['records']=[after['tail']['records'][-1]];after['tail']['records'][0]['sequence']=1
        self.qualify(before,after)

    def test_recovered_drain_metadata_is_strict_and_separately_scoped(self):
        before,after=frames('b'*64)
        final=after['tail']['records'][-1]['final_output_accounting']
        drain=dict(stdout_offset=0,stderr_offset=0,stdout_in_flight_bytes=4096,stderr_in_flight_bytes=0)
        final['recovery_drain']=drain
        output.snapshot(encoded(after),'b'*64)
        with self.assertRaises(ValueError):self.qualify(before,after)

        for mode in ['boolean-offset','float-intent','large-intent','offset-after-final','stderr-after-final','overflow','extra','null']:
            changed=copy.deepcopy(after);d=changed['tail']['records'][-1]['final_output_accounting']['recovery_drain']
            if mode=='boolean-offset':d['stdout_offset']=False
            elif mode=='float-intent':d['stdout_in_flight_bytes']=0.0
            elif mode=='large-intent':d['stdout_in_flight_bytes']=4097
            elif mode=='offset-after-final':d['stdout_offset']=8193
            elif mode=='stderr-after-final':d['stderr_offset']=18
            elif mode=='overflow':d['stdout_offset']=output.U64;d['stdout_in_flight_bytes']=1
            elif mode=='extra':d['token']='private'
            elif mode=='null':changed['tail']['records'][-1]['final_output_accounting']['recovery_drain']=None
            with self.subTest(mode=mode),self.assertRaises(ValueError):output.snapshot(encoded(changed),'b'*64)
        final.clear();final.update(state='unavailable',recovery_drain=drain)
        output.snapshot(encoded(after),'b'*64)
        with self.assertRaises(ValueError):self.qualify(before,after)

    def test_recovered_probe_counters_require_full_undelivered_origin(self):
        before,after=frames('b'*64)
        def qualify():return output.qualify_recovered_pair(encoded(before),encoded(after),'b'*64,b'\0'*8192,output.STDERR,37)
        with self.assertRaises(ValueError):qualify()
        final=after['tail']['records'][-1]['final_output_accounting']
        final['recovery_drain']=dict(stdout_offset=0,stderr_offset=0,stdout_in_flight_bytes=0,stderr_in_flight_bytes=0)
        view=qualify();self.assertEqual(view['scope'],'held-child-recovered-exec-output-accounting')
        self.assertTrue(view['stopped_guest_drain_observed']);self.assertFalse(view['worker_replacement_qualified'])
        self.assertFalse(view['full_g7_g8_qualified'])
        with self.assertRaises(ValueError):self.qualify(before,after)
        for key in final['recovery_drain']:
            final['recovery_drain'][key]=1
            with self.subTest(key=key),self.assertRaises(ValueError):qualify()
            final['recovery_drain'][key]=0

    def test_counter_identity_loss_unknown_and_numeric_attacks_refuse(self):
        def changes(mode,before,after):
            r=after['tail']['records'][-1];b=r['final_output_accounting']['report']['stdout'];d=r['output_disposition']['stdout']
            if mode=='wrong-task':after['task_id']='c'*64
            elif mode=='wrong-generation':after['generation']='b'*32
            elif mode=='wrong-bundle':after['bundle_identity']['inode']+=1
            elif mode=='public-identity':after['bundle_identity']['uid']+=1
            elif mode=='wrong-namespace':after['namespace']='default'
            elif mode=='wrong-sandbox':after['sandbox_id']='other'
            elif mode=='extra-frame':after['payload']='private'
            elif mode=='extra-record':r['payload']='private'
            elif mode=='extra-counter':b['payload']='private'
            elif mode=='extra-disposition':d['payload']='private'
            elif mode=='bad-version':after['version']=2
            elif mode=='boolean-version':after['version']=True
            elif mode=='float-count':after['tail']['deleted_count']=18.0
            elif mode=='boolean-count':after['tail']['deleted_count']=True
            elif mode=='exhausted-tail':after['tail']['accounting_exhausted']=True
            elif mode=='overflow-count':after['tail']['deleted_count']=1<<64
            elif mode=='missed-exec':after['tail']['deleted_count']+=1
            elif mode=='wrong-eviction':after['tail']['evicted_count']+=1
            elif mode=='changed-prefix':after['tail']['records'][0]['exit']=1
            elif mode=='sequence':r['sequence']+=1
            elif mode=='reused-id':r['id']=before['tail']['records'][0]['id'];r['final_output_accounting']['report']['id']=r['id']
            elif mode=='not-docker-id':r['id']='short';r['final_output_accounting']['report']['id']='short'
            elif mode=='pid':r['pid']+=1
            elif mode=='guest-id':r['final_output_accounting']['report']['id']='other'
            elif mode=='missing-final':r.pop('final_output_accounting')
            elif mode=='unavailable-final':r['final_output_accounting']=dict(state='unavailable')
            elif mode=='missing-disposition':r.pop('output_disposition')
            elif mode=='running':r['status']=2
            elif mode=='created':r['status']=1
            elif mode=='exit':r['exit']=0
            elif mode=='boolean-exit':r['exit']=True
            elif mode=='guest-overflow':b['accounting_exhausted']=True
            elif mode=='guest-drop':b['observed_bytes']+=1;b['dropped_bytes']=1;b['truncated']=True
            elif mode=='guest-pending':b['observed_bytes']+=1;b['pending_bytes']=1
            elif mode=='guest-buffered':b['acknowledged_bytes']-=1;b['buffered_bytes']=1
            elif mode=='guest-peak':b['buffered_peak_bytes']=4<<20
            elif mode=='guest-limit':b['limit_bytes']=8<<20
            elif mode=='float-bytes':b['observed_bytes']=float(b['observed_bytes'])
            elif mode=='boolean-bytes':b['observed_bytes']=True
            elif mode=='host-drop':d['written_bytes']-=1;d['pressure_dropped_bytes']=1
            elif mode=='no-destination':d['written_bytes']-=1;d['no_destination_bytes']=1
            elif mode=='unknown-prefix':d['written_bytes']-=1;d['unknown_prefix_bytes']=1
            elif mode=='pending-intent':d['in_flight_bytes']=1
            elif mode=='oversize-intent':d['in_flight_bytes']=4097
            elif mode=='replay':d['replay_exposure_bytes']=1
            elif mode=='exposure-exhausted':d['replay_exposure_bytes']=(1<<64)-1;d['exposure_exhausted']=True
            elif mode=='float-host-bytes':d['written_bytes']=float(d['written_bytes'])
            elif mode=='bad-partition':d['written_bytes']+=1
            elif mode=='bad-offset':r['stdout_offset']+=1
            else:raise AssertionError(mode)
        modes=['wrong-task','wrong-generation','wrong-bundle','public-identity','wrong-namespace','wrong-sandbox','extra-frame','extra-record','extra-counter','extra-disposition','bad-version','boolean-version','float-count','boolean-count','exhausted-tail','overflow-count','missed-exec','wrong-eviction','changed-prefix','sequence','reused-id','not-docker-id','pid','guest-id','missing-final','unavailable-final','missing-disposition','running','created','exit','boolean-exit','guest-overflow','guest-drop','guest-pending','guest-buffered','guest-peak','guest-limit','float-bytes','boolean-bytes','host-drop','no-destination','unknown-prefix','pending-intent','oversize-intent','replay','exposure-exhausted','float-host-bytes','bad-partition','bad-offset']
        for mode in modes:
            with self.subTest(mode=mode):
                before,after=frames('b'*64);changes(mode,before,after)
                with self.assertRaises(ValueError):self.qualify(before,after)
        for kwargs in [dict(stdout=b''),dict(stderr=b'changed'),dict(status=True),dict(status=0)]:
            before,after=frames('b'*64)
            with self.assertRaises(ValueError):self.qualify(before,after,**kwargs)

    def make(self,work,mode=None):
        fixture.ChildLogTests().make(work);p=work/'integration'
        def write(name,value):
            data=value if type(value) is bytes else encoded(value)
            (p/name).write_bytes(data);(p/name).chmod(0o600);return sha(data)
        build=json.loads((work/'build.json').read_text());build.setdefault('binaries',{})['containerd-shim-multikernel-v2']='c'*64
        (work/'build.json').write_bytes(encoded(build));(work/'build.json').chmod(0o600)
        container=(p/'create.stdout').read_text().strip();before,after=frames(container)
        if mode=='loss':after['tail']['records'][-1]['output_disposition']['stdout']['replay_exposure_bytes']=1
        if mode=='wrong-selector':after['generation']='b'*32
        captures={'child-output-before':(encoded(before),b'',0),'child-output-probe':(b'\0'*8192,output.STDERR,37),'child-output-after-0':(encoded(after),b'',0)}
        if mode=='probe-status':captures['child-output-probe']=(b'\0'*8192,output.STDERR,0)
        if mode=='wrong-bytes':captures['child-output-probe']=(b'\0'*8191,output.STDERR,37)
        if mode=='diagnostic':captures['child-output-after-0']=(encoded(after),b'warning',0)
        hashes={}
        for label,(stdout,stderr,status) in captures.items():
            for suffix,data in [('stdout',stdout),('stderr',stderr),('exit.json',dict(returncode=status))]:hashes[label+'.'+suffix]=write(label+'.'+suffix,data)
        for phase in ['before','after']:
            binding=dict(version=1,scope='held-child-output-worker-binding',phase=phase,boot_id=BOOT,source_commit=build['source_commit'],container_id=container,worker_pid=1234,worker_start_ticks=5678,bundle_identity=before['bundle_identity'],executable_sha256='c'*64)
            if mode=='restarted-worker' and phase=='after':binding['worker_start_ticks']+=1
            if mode=='foreign-worker' and phase=='after':binding['worker_pid']+=1
            if mode=='wrong-binary':binding['executable_sha256']='d'*64
            hashes['child-output-'+phase+'-worker.json']=write('child-output-'+phase+'-worker.json',binding)
        view=output.qualify_pair(encoded(frames(container)[0]),encoded(frames(container)[1]),container,b'\0'*8192,output.STDERR,37)
        value=dict(version=1,scope='held-child-output-accounting-checkpoint',boot_id=BOOT,invocation_id=INV,source_commit=build['source_commit'],executable_sha256='c'*64,after_attempt=0,view=view,artifact_sha256=hashes)
        if mode=='wrong-boot':value['boot_id']='0'*8+'-0000-4000-8000-'+'0'*12
        if mode=='wrong-invocation':value['invocation_id']='b'*32
        if mode=='wrong-source':value['source_commit']='b'*40
        if mode=='float-attempt':value['after_attempt']=0.0
        if mode=='extra-checkpoint':value['payload']='private'
        if mode=='false-view':value['view']['docker_log_retention_qualified']=True
        receipt=json.loads((work/'integration-receipt.json').read_text());receipt['child_output_accounting_tested']=mode!='missing-claim';receipt['child_output_accounting_sha256']=write('child-output-accounting.json',value)
        (work/'integration-receipt.json').write_bytes(encoded(receipt))

    def inspect(self,work):
        with patch.object(agent_profile,'ORIGINAL_PROFILE',work/'original-profile.json'):
            return output.inspect_fixture(work,BOOT,INV)

    def test_complete_checkpoint_binds_existing_matrix_and_accounting(self):
        with tempfile.TemporaryDirectory() as raw:
            work=Path(raw);self.make(work);view=self.inspect(work)
            self.assertTrue(view['guest_host_and_receiver_counts_bound']);self.assertFalse(view['full_g7_g8_qualified'])

    def test_recomputed_checkpoint_receipt_attacks_refuse(self):
        for mode in ['loss','wrong-selector','probe-status','wrong-bytes','diagnostic','restarted-worker','foreign-worker','wrong-binary','wrong-boot','wrong-invocation','wrong-source','float-attempt','extra-checkpoint','false-view','missing-claim']:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory() as raw:
                work=Path(raw);self.make(work,mode)
                with self.assertRaises(ValueError):self.inspect(work)

    def test_late_observation_refuses_before_worker_requalification(self):
        before,after=frames('b'*64)
        returns=[subprocess.CompletedProcess([],0,b'before',b''),subprocess.CompletedProcess([],37,b'\0'*8192,output.STDERR),subprocess.CompletedProcess([],0,b'after',b'')]
        with patch.object(output,'read_fixture_artifact',return_value=encoded({'source_commit':'a'*40})),patch.object(output,'worker_binding',return_value=(Path('/synthetic'),{'container_id':'b'*64,'bundle_identity':before['bundle_identity']})) as binding,patch.object(output,'capture',side_effect=returns),patch.object(output,'snapshot',return_value=before),patch.object(output.time,'monotonic_ns',side_effect=[0,0,0,0,5_000_000_001]):
            with self.assertRaises(ValueError):output.qualify([],Path('/synthetic'),BOOT,INV)
            self.assertEqual(binding.call_count,1)

    def test_frozen_clock_cannot_expand_the_poll_count(self):
        before,_=frames('b'*64)
        initial=[subprocess.CompletedProcess([],0,b'before',b''),subprocess.CompletedProcess([],37,b'\0'*8192,output.STDERR)]
        initial += [subprocess.CompletedProcess([],0,b'before',b'') for _ in range(25)]
        with patch.object(output,'read_fixture_artifact',return_value=encoded({'source_commit':'a'*40})),patch.object(output,'worker_binding',return_value=(Path('/synthetic'),{'container_id':'b'*64,'bundle_identity':before['bundle_identity']})),patch.object(output,'capture',side_effect=initial) as capture,patch.object(output,'snapshot',return_value=before),patch.object(output.time,'monotonic_ns',return_value=0),patch.object(output.time,'sleep'):
            with self.assertRaises(ValueError):output.qualify([],Path('/synthetic'),BOOT,INV)
            self.assertEqual(capture.call_count,27)

if __name__=='__main__':unittest.main()
