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
        return dict(version=1, scope='durable-ownership-and-kernel-presence',
                    status='consistent', boot_id=BOOT, boot_stable=True, revision='45e784f',
                    ownership=dict(version=1, source='durable-lifecycle', sequence=4, sandboxes=[OWNER]),
                    kernel_instances=['box-a'], kernel_generation_verified=False,
                    findings=[], unobserved=UNOBSERVED)

    def test_running_and_exact_generation_release(self):
        r=self.report()
        self.assertEqual(qualify(json.dumps(r).encode(), BOOT, OWNER), r)
        for retained in [[], [{**OWNER, 'state': 'ABSENT'}]]:
            r['ownership']['sandboxes']=retained
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

    def test_ambiguous_oversized_and_incomplete_cleanup_refuses(self):
        raw=json.dumps(self.report()).encode()
        with self.assertRaises(ValueError):qualify(b'{"version":1,'+raw[1:], BOOT, OWNER)
        with self.assertRaises(ValueError):qualify(b' '*(1 << 20)+raw, BOOT, OWNER)
        with self.assertRaises(ValueError):qualify(raw, BOOT, OWNER, released=True)
        r=self.report();r['kernel_instances']=[]
        r['ownership']['sandboxes']=[{**OWNER,'state':'ABSENT','generation':'b'*32}]
        with self.assertRaises(ValueError):qualify(json.dumps(r).encode(), BOOT, OWNER, released=True)


if __name__ == '__main__':unittest.main()
