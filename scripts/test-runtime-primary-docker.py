#!/usr/bin/env python3
import json
import sys
import time
import unittest
from runtime_primary_docker import observe, unchanged, bounded_command, ENV, CLI

ID = 'sha256:' + 'a'*64
OTHER = 'sha256:' + 'b'*64


class PrimaryDockerTests(unittest.TestCase):
    def response(self, tag='private-tag', image=ID, reverse=False):
        calls = []
        def command(args):
            self.assertEqual(args[:3], CLI)
            calls.append(args)
            if args[3] == 'info':
                return b'"primary-engine"\n'
            if args[3] == 'image':
                rows = [{'ID':image,'Repository':'private-repository','Tag':tag,'Digest':'<none>', 'CreatedSince':'volatile'}]
                return ('\n'.join(json.dumps(row) for row in rows)+'\n').encode()
            rows = {'container': ['c'*64, 'd'*64], 'volume':['private-volume'], 'network':['e'*64]}[args[3]]
            return ('\n'.join(reversed(rows) if reverse else rows)+'\n').encode()
        return command, calls

    def test_stable_inventory_hashes_without_names_or_order_dependence(self):
        left, calls = self.response();right, _ = self.response(reverse=True)
        a=observe(command=left);b=observe(command=right)
        unchanged(a,b)
        self.assertEqual(len(calls),5)
        encoded=json.dumps(a)
        for private in ['private-repository','private-tag','private-volume',ID,'primary-engine']:
            self.assertNotIn(private,encoded)
        self.assertEqual(a['inventories']['containers']['count'],2)
        self.assertNotIn('DOCKER_CONTEXT',ENV)
        self.assertEqual(ENV['DOCKER_CONFIG'],'/proc/self/fd/-1')

    def test_tag_only_change_and_child_image_presence_are_detected(self):
        left,_=self.response();right,_=self.response(tag='other-tag')
        with self.assertRaises(ValueError):unchanged(observe(command=left),observe(command=right))
        self.assertFalse(observe(ID,command=left)['child_image_absent'])
        self.assertTrue(observe(OTHER,command=left)['child_image_absent'])
        with self.assertRaises(ValueError):observe('not-an-image',command=left)

    def test_bad_or_ambiguous_metadata_is_refused(self):
        base,_=self.response()
        for payload in [b'{}', b'{"ID":"x","ID":"y"}', b'{"ID":null}', b'not-json']:
            def command(args):return payload if args[3]=='image' else base(args)
            with self.subTest(payload=payload),self.assertRaises((ValueError,KeyError,TypeError)):
                observe(command=command)
        def duplicates(args):return (('c'*64+'\n')*2).encode() if args[3]=='container' else base(args)
        with self.assertRaises(ValueError):observe(command=duplicates)

    def test_command_errors_and_pressure_are_bounded_without_echoing_payload(self):
        for body, kwargs in [
                ("print('sensitive-command-error');raise SystemExit(1)",{}),
                ("print('x'*100000)",{'limit':1024}),
                ("import time;time.sleep(30)",{'timeout':.05}),
                ("import os,time;os.close(1);os.close(2);time.sleep(30)",{'timeout':.05})]:
            started=time.monotonic()
            with self.assertRaises(RuntimeError) as error:
                bounded_command([sys.executable,'-c',body],**kwargs)
            self.assertNotIn('sensitive-command-error',str(error.exception))
            self.assertLess(time.monotonic()-started,3)
        with self.assertRaises(RuntimeError):bounded_command([sys.executable,'-c','pass'],timeout=0)


if __name__=='__main__':unittest.main()
