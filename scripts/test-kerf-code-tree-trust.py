#!/usr/bin/env python3
"""Code-tree permission hardening: rejection, immutability and write controls."""
import importlib.util
import os
from pathlib import Path
import sys
import tempfile
sys.dont_write_bytecode=True
s=importlib.util.spec_from_file_location('kerf_tree',Path(__file__).with_name('harden-kerf-installation.py'));m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
with tempfile.TemporaryDirectory(prefix='mk-kerf-tree-')as raw:
 t=Path(raw);tree=t/'source';tree.mkdir(mode=0o775);tree.chmod(0o775)
 child=tree/'kerf';child.mkdir(mode=0o775);child.chmod(0o775)
 p=child/'module.py';p.write_bytes(b'fixture code\n');p.chmod(0o664)
 before=m.inspect(tree,os.getuid());after,n=m.apply(tree,before,os.getuid())
 assert n==3 and m.content_digest(before)==m.content_digest(after)
 assert p.read_bytes()==b'fixture code\n'and p.stat().st_mode&0o777==0o644
 assert tree.stat().st_mode&0o777==0o755 and child.stat().st_mode&0o777==0o755
 # Parent source symlinks cannot authorize chmod outside the inspected tree.
 external=t/'external';external.mkdir();external.chmod(0o755);f=external/'other.py';f.write_bytes(b'outside');f.chmod(0o600)
 (tree/'external-link').symlink_to(external,target_is_directory=True)
 b=m.inspect(tree,os.getuid());m.apply(tree,b,os.getuid());assert f.stat().st_mode&0o777==0o600
 (tree/'external-link').unlink()
 # A writable intermediate alias is refused even when it currently resolves
 # to the protected external directory.
 alias=t/'writable-alias';alias.mkdir();alias.chmod(0o777)
 (alias/'hop').symlink_to(external,target_is_directory=True)
 (tree/'indirect').symlink_to(alias/'hop',target_is_directory=True)
 try:m.inspect(tree,os.getuid())
 except ValueError:pass
 else:raise AssertionError('replaceable intermediate symlink trusted')
 (tree/'indirect').unlink()
 # Content replacement after the plan must fail before chmod of the changed file.
 p.chmod(0o664);b=m.inspect(tree,os.getuid());p.write_bytes(b'modified')
 try:m.apply(tree,b,os.getuid())
 except ValueError:pass
 else:raise AssertionError('changed code accepted')
 assert p.stat().st_mode&0o777==0o664
 # Hard links and special files are rejected before applying a plan.
 os.link(p,tree/'alias')
 try:m.inspect(tree,os.getuid())
 except ValueError:pass
 else:raise AssertionError('hard link accepted')
 (tree/'alias').unlink();fifo=tree/'fifo';os.mkfifo(fifo)
 try:m.inspect(tree,os.getuid())
 except ValueError:pass
 else:raise AssertionError('FIFO accepted')
 fifo.unlink();link=t/'tree-link';link.symlink_to(tree,target_is_directory=True)
 try:m.inspect(link,os.getuid())
 except ValueError:pass
 else:raise AssertionError('symlink root accepted')
print('Kerf imported-code permission boundary tests: PASS')
