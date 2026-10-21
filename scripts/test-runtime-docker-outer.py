#!/usr/bin/env python3
"""Test data-only Docker helper closure preparation and loader isolation."""
import hashlib
import json
import os
from pathlib import Path
import runpy
import stat
import struct
import tempfile
import unittest

module=runpy.run_path(str(Path(__file__).with_name('validate-runtime-docker-root.py')))
Error=module['ApprovalError']

def elf(interpreter=None, needed=(), extra=()):
    strings=b'\0';indices=[]
    for name in needed:
        indices.append(len(strings));strings+=name.encode()+b'\0'
    dynamic=[(1,index) for index in indices]+list(extra)
    if needed:dynamic += [(5,0x400800),(10,len(strings))]
    dynamic += [(0,0)]
    data=bytearray(4096)
    data[:16]=b'\x7fELF\x02\x01\x01'+b'\0'*9
    count=1+bool(interpreter)+bool(needed or extra)
    data[16:64]=struct.pack('<HHIQQQIHHHHHH',3,62,1,0,64,0,0,64,56,count,0,0,0)
    records=[(1,5,0,0x400000,0,len(data),len(data),4096)]
    if interpreter:
        text=interpreter.encode()+b'\0';data[0x300:0x300+len(text)]=text
        records.append((3,4,0x300,0,0,len(text),len(text),1))
    if needed or extra:
        table=b''.join(struct.pack('<qQ',*value) for value in dynamic)
        data[0x500:0x500+len(table)]=table;data[0x800:0x800+len(strings)]=strings
        records.append((2,4,0x500,0,0,len(table),len(table),8))
    for i,record in enumerate(records):data[64+i*56:64+(i+1)*56]=struct.pack('<IIQQQQQQ',*record)
    return bytes(data)

class OuterTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.base=Path(self.tmp.name);self.root=self.base/'image';self.root.mkdir()
        self.files={name:elf('/lib/ld-musl-x86_64.so.1',['libc.so']) for name in module['HELPERS']}
        self.files['lib/ld-musl-x86_64.so.1']=elf()
        self.write()
        (self.root/'usr/lib').mkdir(parents=True,exist_ok=True)
        (self.root/'usr/lib/libc.so').symlink_to('/lib/ld-musl-x86_64.so.1')
    def write(self):
        for name,data in self.files.items():
            p=self.root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(data);p.chmod(0o755)
    def scan(self):
        entries=[]
        for p in [self.root,*sorted(self.root.rglob('*'))]:
            s=p.lstat();item={'path':str(p.relative_to(self.root)),'mode':s.st_mode,'uid':s.st_uid,'gid':s.st_gid,'size':s.st_size}
            if p.is_symlink():item.update(type='symlink',target=os.readlink(p))
            elif p.is_dir():item.update(type='directory')
            else:item.update(type='regular',sha256=hashlib.sha256(p.read_bytes()).hexdigest())
            entries.append(item)
        return json.dumps({'schema_version':1,'entries':entries}).encode()
    def prepare(self,raw=None):
        return module['prepare_outer'](self.root,self.scan() if raw is None else raw,self.base/'outer',required_uid=os.geteuid())
    def test_independent_closure_and_fixed_accounts(self):
        result=self.prepare();self.assertTrue(result['independent_code'])
        out=self.base/'outer';self.assertEqual(result['files'],5)
        for p in out.rglob('*'):
            self.assertFalse(p.is_symlink());self.assertEqual(p.stat().st_uid,os.geteuid())
            if p.is_dir():self.assertEqual(stat.S_IMODE(p.stat().st_mode),0o755)
        for name in module['HELPERS']:
            p=out/name;self.assertEqual(p.read_bytes(),self.files[name]);self.assertNotEqual(p.stat().st_ino,(self.root/name).stat().st_ino)
            self.assertEqual(stat.S_IMODE(p.stat().st_mode),module['HELPERS'][name])
        self.assertEqual((out/'etc/subuid').read_text(),'rootless:100000:65536\n')
        self.assertNotIn('etc/ld.so.cache',result['file_sha256'])
        (self.root/'usr/bin/newuidmap').write_text('untrusted later replacement')
        self.assertEqual((out/'usr/bin/newuidmap').read_bytes(),self.files['usr/bin/newuidmap'])
    def test_changed_content_refused_before_output(self):
        raw=self.scan();(self.root/'usr/bin/newuidmap').write_bytes(b'x'*4096)
        with self.assertRaises(Error):self.prepare(raw)
        self.assertFalse((self.base/'outer').exists())
    def test_symlink_substitution_refused(self):
        raw=self.scan();p=self.root/'usr/bin/newuidmap';p.unlink();p.symlink_to('/bin/true')
        with self.assertRaises(OSError):self.prepare(raw)
    def test_helper_mode_owner_and_missing_library_refused(self):
        for mutation in ['writable','owner','absent']:
            with self.subTest(mutation=mutation):
                raw=json.loads(self.scan())
                if mutation=='absent':raw['entries']=[e for e in raw['entries'] if e['path']!='usr/lib/libc.so']
                else:
                    entry=next(e for e in raw['entries'] if e['path']=='usr/bin/newuidmap')
                    if mutation=='writable':entry['mode']|=0o020
                    else:entry['uid']+=1
                with self.assertRaises(Error):self.prepare(json.dumps(raw).encode())
                self.assertFalse((self.base/'outer').exists())
    def test_virtual_alias_resolution_and_escape(self):
        paths={'usr':{'type':'directory'},'usr/lib':{'type':'directory'},'usr/lib/libc.so':{'type':'regular'},'lib':{'type':'symlink','target':'usr/lib'}}
        self.assertEqual(module['resolve_image_path'](paths,'lib/libc.so'),'usr/lib/libc.so')
        for target in ['../outside','/bundle/rootfs/usr/lib','lib']:
            paths['lib']['target']=target
            with self.assertRaises(Error):module['resolve_image_path'](paths,'lib/libc.so')
    def test_rootlesskit_approved_build_uid_is_data_only(self):
        paths={entry['path']:entry for entry in json.loads(self.scan())['entries']}
        fd=os.open(self.root,os.O_RDONLY|os.O_DIRECTORY)
        self.addCleanup(os.close,fd)
        # Even with root required for privileged helpers, a non-setuid
        # rootlesskit release can retain its approved build UID on source.
        self.assertEqual(module['read_image_file'](fd,paths,'usr/local/bin/rootlesskit',0),self.files['usr/local/bin/rootlesskit'])
        if os.geteuid()!=0:
            with self.assertRaises(Error):module['read_image_file'](fd,paths,'usr/bin/newuidmap',0)
    def test_destination_never_adopted(self):
        out=self.base/'outer';out.mkdir();(out/'sentinel').write_text('keep')
        with self.assertRaises(FileExistsError):self.prepare()
        self.assertEqual((out/'sentinel').read_text(),'keep')
    def test_dynamic_loader_metadata_policy(self):
        parse=module['elf_dependencies']
        self.assertEqual(parse(elf()),(None,[]))
        self.assertEqual(parse(elf('/lib/ld-musl-x86_64.so.1',['libc.so'])),('/lib/ld-musl-x86_64.so.1',['libc.so']))
        for data in [b'#!/bin/sh\nexit 0\n',elf('/bundle/rootfs/loader'),elf(needed=['../libc.so']),elf(needed=['/lib/libc.so']),elf(extra=[(15,0)]),elf(extra=[(29,0)]),elf(extra=[(0x6ffffefc,0)]),elf(extra=[(0x7fffffff,0)])]:
            with self.assertRaises(Error):parse(data)
        for cut in [1,63,90,1000]:
            with self.assertRaises(Error):parse(elf()[:cut])
    def test_custom_loader_path_refused_before_output(self):
        self.files['usr/bin/newuidmap']=elf(extra=[(29,0)])
        self.write()
        with self.assertRaises(Error):self.prepare()
        self.assertFalse((self.base/'outer').exists())

if __name__=='__main__':unittest.main()
