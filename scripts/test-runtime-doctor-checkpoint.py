#!/usr/bin/env python3
import copy
import json
import unittest
from runtime_doctor_checkpoint import qualify, UNOBSERVED

BOOT = '00ac7aff-a963-4e30-a085-6811e83f0e12'
OWNER = dict(sandbox_id='box-a', generation='a'*32, state='RUNNING', cpus=[8,9],
             memory_bytes=3 << 30, child_cid=40, agent_port=1000)


class DoctorCheckpointTests(unittest.TestCase):
    def report(self):
        report = dict(version=2, scope='durable-resource-ownership-and-kernel-presence',
                    status='consistent', boot_id=BOOT, boot_stable=True, revision='45e784f',
                    ownership=dict(version=1, source='durable-lifecycle', sequence=4, sandboxes=[OWNER]),
                    kernel_instances=['box-a'], kernel_generation_verified=False,
                    findings=[], unobserved=UNOBSERVED)
        descriptor=dict(sandbox_id=OWNER['sandbox_id'],generation=OWNER['generation'],export_generation='b'*32,descriptor_sha256='c'*64)
        lease=dict(sandbox_id=OWNER['sandbox_id'],sandbox_generation=OWNER['generation'],export_generation='b'*32,peer_cid=OWNER['child_cid'],state='ACTIVE',filesystem_uuid='11111111-2222-4333-8444-555555555555',size_bytes=64<<20,quota_bytes=64<<20,inode_limit=4096,port=4061,image_device=1,image_inode=2,offline_check='',counters=dict(reads=0,read_bytes=0,writes=0,written_bytes=0,flushes=0))
        report['resources']=dict(version=1,source='durable-lifecycle-storage',lifecycle=report['ownership'],storage_bindings=[dict(descriptor,required=True)],storage=dict(version=1,source='durable-storage',exports=[lease]),storage_descriptors=[descriptor])
        return report

    def test_running_and_exact_generation_release(self):
        r=self.report()
        self.assertEqual(qualify(json.dumps(r).encode(), BOOT, OWNER), r)
        for retained in [[], [{**OWNER, 'state': 'ABSENT'}]]:
            r['ownership']['sandboxes']=retained
            r['resources']['storage_bindings']=[] if retained==[] else [dict(r['resources']['storage_descriptors'][0],required=True)]
            r['resources']['storage']['exports'][0]['state']='RELEASED'
            r['resources']['storage']['exports'][0]['offline_check']='e2fsck-clean-sha256:'+'d'*64
            r.pop('kernel_instances', None)
            self.assertEqual(qualify(json.dumps(r).encode(), BOOT, OWNER, released=True), r)

    def test_wrong_scope_boot_generation_or_inventory_refuses(self):
        changes=[('status','inconclusive'), ('boot_stable',False), ('boot_id','different'),
                 ('kernel_generation_verified',True), ('scope','complete-health'),
                 ('revision','unknown'), ('findings',[{'code':'ORPHAN_CANDIDATE'}]),
                 ('unobserved',[]), ('kernel_instances',[]), ('kernel_instances',['foreign']),
                 ('version',True), ('private-token','SECRET')]
        for key,value in changes:
            r=self.report();r[key]=value
            with self.subTest(key=key), self.assertRaises(ValueError):qualify(json.dumps(r).encode(), BOOT, OWNER)
        for key,value in [('generation','b'*32), ('child_cid',41), ('cpus',[12,13]), ('state','ABSENT')]:
            r=copy.deepcopy(self.report());r['ownership']['sandboxes'][0][key]=value
            with self.subTest(key=key), self.assertRaises(ValueError):qualify(json.dumps(r).encode(), BOOT, OWNER)
        r=self.report();r['ownership']['sequence']=True
        with self.assertRaises(ValueError):qualify(json.dumps(r).encode(), BOOT, OWNER)

    def test_storage_correlation_mismatch_refuses(self):
        for mode in ['peer','generation','descriptor','missing','quota','state','partial-release']:
            r=self.report();resources=r['resources'];lease=resources['storage']['exports'][0]
            if mode=='peer':lease['peer_cid']+=1
            elif mode=='generation':lease['sandbox_generation']='f'*32
            elif mode=='descriptor':resources['storage_bindings'][0]['descriptor_sha256']='e'*64
            elif mode=='missing':resources['storage']['exports']=[]
            elif mode=='quota':lease['quota_bytes']-=1
            elif mode=='state':lease['state']='QUIESCING'
            elif mode=='partial-release':lease['offline_check']='e2fsck-clean-sha256:'+'d'*64
            with self.subTest(mode=mode),self.assertRaises(ValueError):qualify(json.dumps(r).encode(),BOOT,OWNER)

    def test_ambiguous_oversized_and_incomplete_cleanup_refuses(self):
        raw=json.dumps(self.report()).encode()
        with self.assertRaises(ValueError):qualify(b'{"version":1,'+raw[1:], BOOT, OWNER)
        with self.assertRaises(ValueError):qualify(b' '*(1 << 20)+raw, BOOT, OWNER)
        with self.assertRaises(ValueError):qualify(raw, BOOT, OWNER, released=True)
        r=self.report();r['kernel_instances']=[]
        r['ownership']['sandboxes']=[{**OWNER,'state':'ABSENT','generation':'b'*32}]
        with self.assertRaises(ValueError):qualify(json.dumps(r).encode(), BOOT, OWNER, released=True)


if __name__ == '__main__':unittest.main()
