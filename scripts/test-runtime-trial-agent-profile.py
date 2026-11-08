#!/usr/bin/env python3
"""Private profile derivation policy; external validator/build are controlled."""
import hashlib,json,os,subprocess as sp,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_trial_agent_profile as subject

class AgentProfile(unittest.TestCase):
 def setUp(self):self.previous_umask=os.umask(0o077)
 def tearDown(self):os.umask(self.previous_umask)
 def test_derivation_validates_both_profiles_and_excludes_ambient_credential(self):
  for mode in ['valid','pin-drift','original-refused','derived-refused']:
   with self.subTest(mode=mode),tempfile.TemporaryDirectory() as directory:
    work=Path(directory);(work/'bin').mkdir();private=work/'integration';private.mkdir(mode=0o700);(private/'manifests').mkdir(mode=0o700)
    agent=work/'bin/mk-agent';agent.write_bytes(b'controlled agent fixture');agent.chmod(0o500)
    (work/'build.json').write_text(json.dumps(dict(source_commit='a'*40,binaries={'mk-agent':subject.sha(agent)})))
    original=work/'original.json';profile=dict(transport=dict(module=dict(path='/approved/module')),relay=dict(path='/approved/relay'),agent=dict(path='old',sha256='old'),initramfs=dict(path='old',sha256='old'));original.write_text(json.dumps(profile));original.chmod(0o600)
    before=original.read_bytes();approval=private/'manifests/gce-mk2.json';calls=[]
    def controlled(args,**kw):
     calls.append((args,kw));self.assertFalse(any(str(a).startswith('--skip') for a in args))
     if args[0]=='/bin/bash':
      self.assertNotIn('MK_AGENT_TOKEN_FILE',kw['env']);self.assertEqual(kw['env']['BUSYBOX'],'/bin/busybox');(private/'current-agent.cpio.gz').write_bytes(b'controlled archive fixture');return sp.CompletedProcess(args,0,b'build',b'')
     failed=mode=='original-refused' and len(calls)==1 or mode=='derived-refused' and len(calls)==3
     return sp.CompletedProcess(args,1 if failed else 0,b'validation',b'')
    if mode=='pin-drift':agent.chmod(0o700);agent.write_bytes(b'drift');agent.chmod(0o500)
    with patch.object(subject.sp,'run',controlled),patch.dict(os.environ,{'MK_AGENT_TOKEN_FILE':'/private/ambient-token'}):
     if mode=='valid':
      derived=subject.prepare(work,original,approval);self.assertEqual(len(calls),3);self.assertEqual(derived['agent']['sha256'],subject.sha(agent));self.assertEqual(derived['transport'],profile['transport'])
     else:
      with self.assertRaises(ValueError):subject.prepare(work,original,approval)
    self.assertEqual(original.read_bytes(),before)
    self.assertEqual(len(calls),{'valid':3,'pin-drift':0,'original-refused':1,'derived-refused':3}[mode])
    if mode in ['pin-drift','original-refused']:self.assertFalse(approval.exists())
if __name__=='__main__':unittest.main()
