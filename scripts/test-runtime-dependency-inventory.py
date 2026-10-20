#!/usr/bin/env python3
"""Refusal and redaction boundaries of the dependency collector."""
from pathlib import Path
import importlib.util
import json
import os
import stat
import subprocess
import tempfile
import sys

sys.dont_write_bytecode=True
root=Path(__file__).resolve().parent.parent
spec=importlib.util.spec_from_file_location('dependency_inventory',root/'scripts/collect-runtime-dependencies.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
with tempfile.TemporaryDirectory(prefix='mk-dependency-inventory-')as raw:
 t=Path(raw);p=t/'regular';p.write_bytes(b'public artifact')
 a=m.file_record(p);assert a['size_bytes']==15 and len(a['sha256'])==64
 # Ordinary caller-owned fixture paths must never be reported as root-protected.
 if os.getuid()!=0:assert not a['protected_root_owned']
 fifo=t/'fifo';os.mkfifo(fifo)
 try:m.file_record(fifo)
 except ValueError:pass
 else:raise AssertionError('FIFO accepted')
 try:m.file_record(p,maximum=2)
 except ValueError:pass
 else:raise AssertionError('oversized file accepted')
 original_read=m.os.read;count=[0]
 def changed(fd,n):
  b=original_read(fd,n)
  if not count[0]:
   count[0]+=1;p.write_bytes(b'changed during hashing')
  return b
 m.os.read=changed
 try:
  try:m.file_record(p)
  except ValueError:pass
  else:raise AssertionError('mutation during hashing accepted')
 finally:m.os.read=original_read
 # Exercise emitted metadata with a synthetic direct-reference credential.
 d=t/'fixture-1.0.dist-info';d.mkdir();(d/'METADATA').write_text('Metadata-Version: 2.1\nName: fixture\nVersion: 1.0\nLicense: MIT\nRequires-Dist: dependency @ https://public-user:synthetic-secret@invalid.example/pkg\n')
 unknown=t/'unknown-1.0.dist-info';unknown.mkdir();(unknown/'METADATA').write_text('Metadata-Version: 2.1\nName: unknown-fixture\nVersion: 1.0\nLicense: UNKNOWN\n')
 code='import sys;sys.path.insert(0,'+repr(str(t))+')\n'+m.PYTHON_METADATA
 r=subprocess.run([sys.executable,'-I','-c',code],capture_output=True,text=True,check=True)
 assert 'synthetic-secret'not in r.stdout and 'public-user'not in r.stdout and 'invalid.example'not in r.stdout
 row=next(x for x in json.loads(r.stdout)if x['name']=='fixture')
 assert row['requires_names']==['dependency']and row['direct_reference_count']==1
 unknown_row=next(x for x in json.loads(r.stdout)if x['name']=='unknown-fixture')
 assert unknown_row['declared_license']is None and unknown_row['license_status']=='metadata_requires_review'
 assert m.public('MIT')=='MIT' and m.public('https://user:secret@example.invalid')is None
spec=importlib.util.spec_from_file_location('dependency_summary',root/'scripts/summarize-runtime-dependencies.py')
z=importlib.util.module_from_spec(spec);spec.loader.exec_module(z)
data={'schema_version':1,'review_status':'REQUIRES_REVIEW','artifacts':[{'family':'runtime-go','path':'private-path-sentinel','resolved':'private-path-sentinel','software':'identity-sentinel','version':'version-sentinel','sha256':'artifact-hash-sentinel','protected_root_owned':True,'matches_expected':True}],
 'collection_gaps':[],'selected_package_licenses':{'package-sentinel':{'status':'declared_labels','copyright':{'path':'copyright-path-sentinel'}}},
 'python_distributions':[{'name':'python-name-sentinel','version':'python-version-sentinel','license_status':'declared','declared_license':'license-sentinel','direct_reference_count':1}],
 'support':{'files':[{'matches_expected':True,'protected_root_owned':True,'name':'support-name-sentinel'}]},'installed_packages':[{'package':'package-sentinel'}],
 'binary_metadata':[{'go_status':'available','dependencies':[{'path':'module-path-sentinel','version':'module-version-sentinel'}]}],
 'kerf_checkout':{'matches_profile':True,'tracked_tree_modified':False},'services':[{'service':'service-sentinel'}],'coverage_limits':['limit-sentinel']}
encoded=json.dumps(z.summary(data,'1'*64));assert 'sentinel'not in encoded
print('runtime dependency inventory boundary tests: PASS')
