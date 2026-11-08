#!/usr/bin/env python3
"""Real files and adversarial controls for cross-boot private retention."""
import copy,json,os,tempfile,unittest
from pathlib import Path
from unittest.mock import patch
import runtime_trial_retention as subject
BOOT='94779fd9-6dd3-4bdf-8064-6149caccf1c5';NEXT='509d5e93-31b8-4e8f-925a-4caa9fcf4788'

class Retention(unittest.TestCase):
 def setUp(self):self.mask=os.umask(0o077)
 def tearDown(self):os.umask(self.mask)
 def fixture(self,base):
  p=Path(base);p.chmod(0o700);a=p/'receipt.json';a.write_bytes(b'{"controlled":"synthetic"}\n');empty=p/'empty.stderr';empty.write_bytes(b'');return [str(a),str(empty)]
 def test_roundtrip_changed_boot_and_exclusive_publication(self):
  with tempfile.TemporaryDirectory() as d:
   paths=self.fixture(d);m=subject.capture(paths,BOOT);destination=str(Path(d)/'retention.json');subject.publish(destination,m)
   self.assertEqual(Path(destination).stat().st_mode&0o777,0o600);self.assertEqual(subject.load(destination),m)
   flags=subject.verify_after_boot(m,NEXT);self.assertTrue(flags['artifacts_preserved']);self.assertFalse(flags['prior_completion_accepted']);self.assertFalse(flags['cold_restoration_qualified'])
   with self.assertRaises(ValueError):subject.verify_after_boot(m,BOOT)
   with self.assertRaises(FileExistsError):subject.publish(destination,m)
   self.assertEqual(subject.load(destination),m)
 def test_bytes_metadata_inode_and_link_drift_refuse(self):
  for mode in ['bytes','mode','inode','hardlink','symlink','parent-symlink','oversized','fifo']:
   with self.subTest(mode=mode),tempfile.TemporaryDirectory() as d:
    paths=self.fixture(d);m=subject.capture(paths,BOOT);p=Path(paths[0]);original=p.read_bytes()
    if mode=='bytes':p.write_bytes(b'different')
    elif mode=='mode':p.chmod(0o640)
    elif mode=='inode':q=p.with_name('replacement');q.write_bytes(original);q.replace(p)
    elif mode=='hardlink':os.link(p,p.with_name('linked'))
    elif mode=='symlink':p.unlink();p.symlink_to(paths[1])
    elif mode=='parent-symlink':
     q=Path(d)/'nested';q.mkdir(mode=0o700);p.rename(q/p.name);(Path(d)/'alias').symlink_to(q,target_is_directory=True);m['artifacts'][str(Path(d)/'alias'/p.name)]=m['artifacts'].pop(str(p))
    elif mode=='oversized':
     with p.open('wb') as f:f.truncate(subject.MAX_FILE_BYTES+1)
    elif mode=='fifo':p.unlink();os.mkfifo(p,0o600)
    with self.assertRaises((ValueError,OSError)):subject.verify_after_boot(m,NEXT)
 def test_explicit_list_and_strict_manifest(self):
  with tempfile.TemporaryDirectory() as d:
   paths=self.fixture(d);m=subject.capture(paths,BOOT)
   for bad in [[],paths*2,[paths[0]+'/../receipt.json'],[str(Path(d))+'//receipt.json'],[paths[0]+'\n'],[True]]:
    with self.assertRaises((ValueError,OSError)):subject.capture(bad,BOOT)
   for change in ['version','extra','bool-size','missing-sha','unsafe-mode','foreign-uid','empty','boot']:
    n=copy.deepcopy(m);record=n['artifacts'][paths[0]]
    if change=='version':n['version']=True
    elif change=='extra':n['output']='secret'
    elif change=='bool-size':record['bytes']=False
    elif change=='missing-sha':record.pop('sha256')
    elif change=='unsafe-mode':record['mode']=0o666
    elif change=='foreign-uid':record['uid']=os.geteuid()+1
    elif change=='empty':n['artifacts']={}
    elif change=='boot':n['previous_boot_id']='invented'
    with self.assertRaises(ValueError):subject.validate(n)
 def test_duplicate_json_and_unsafe_publication_parent_refuse(self):
  with tempfile.TemporaryDirectory() as d:
   paths=self.fixture(d);m=subject.capture(paths,BOOT);p=Path(d)/'manifest.json';p.write_text('{"version":1,"version":1}');p.chmod(0o600)
   with self.assertRaises(ValueError):subject.load(str(p))
   q=Path(d)/'public';q.mkdir(mode=0o755);q.chmod(0o755)
   with self.assertRaises(ValueError):subject.publish(str(q/'new.json'),m)
   self.assertFalse((q/'new.json').exists())
 def test_whole_set_drift_is_reobserved(self):
  with tempfile.TemporaryDirectory() as d:
   paths=self.fixture(d);real=subject.read_artifact;count=0
   def changing(path,*args):
    nonlocal count
    result=real(path,*args);count+=1
    if count==2:Path(paths[0]).write_bytes(b'drift during later artifact read')
    return result
   with patch.object(subject,'read_artifact',changing),self.assertRaises(ValueError):subject.capture(paths,BOOT)
 def test_explicit_absences_cannot_be_filled_or_overlap(self):
  with tempfile.TemporaryDirectory() as d:
   paths=self.fixture(d);missing=str(Path(d)/'failed-phase-checkpoint.json')
   m=subject.capture(paths,BOOT,[missing]);self.assertEqual(subject.verify_after_boot(m,NEXT)['absent_artifact_count'],1)
   for bad in [[missing,missing],[paths[0]],[False]]:
    with self.assertRaises(ValueError):subject.capture(paths,BOOT,bad)
   Path(missing).symlink_to(Path(d)/'also-absent')
   with self.assertRaises(ValueError):subject.verify_after_boot(m,NEXT)
   Path(missing).unlink();Path(missing).write_text('forged checkpoint')
   with self.assertRaises(ValueError):subject.verify_after_boot(m,NEXT)
if __name__=='__main__':unittest.main()
