#!/usr/bin/env python3
import json,sys,tempfile,time,unittest
from pathlib import Path
from runtime_trial_docker_command import capture

class DockerCommandTests(unittest.TestCase):
    def run_command(self,root,code,index=0,deadline=None):
        return capture([sys.executable,'-c',code],root,index,deadline or time.monotonic_ns()+2_000_000_000,dict(PATH='/usr/bin:/bin',LANG='C'))

    def test_warning_is_retained_privately_and_does_not_corrupt_identity(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);value=self.run_command(root,'import sys;print("a"*64);print("synthetic private warning",file=sys.stderr)')
            self.assertEqual(value,b'a'*64+b'\n');self.assertEqual((root/'command-00.stderr').read_bytes(),b'synthetic private warning\n')
            self.assertIsNone(json.loads((root/'command-00.json').read_text())['failure'])
            self.assertTrue(all(p.stat().st_mode&0o777==0o600 for p in root.iterdir()))

    def test_failed_command_and_stream_flood_retain_bounded_refusal(self):
        for index,code,failure in [(0,'import sys;print("private failure",file=sys.stderr);sys.exit(7)','exit-status'),(1,'import os;os.write(1,b"x"*(1<<20))','stream-limit')]:
            with tempfile.TemporaryDirectory() as raw:
                root=Path(raw)
                with self.assertRaisesRegex(ValueError,'^private Docker trial command failed$'):self.run_command(root,code,index)
                record=json.loads((root/f'command-{index:02d}.json').read_text());self.assertEqual(record['failure'],failure)
                self.assertLessEqual(record['stdout_bytes'],65537);self.assertLessEqual(record['stderr_bytes'],4097)

    def test_existing_evidence_and_expired_budget_do_not_execute(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);marker=root/'must-not-exist';code='from pathlib import Path;Path('+repr(str(marker))+').touch()'
            with self.assertRaises(ValueError):self.run_command(root,code,deadline=time.monotonic_ns()-1)
            self.assertFalse(marker.exists());original=(root/'command-00.json').read_bytes()
            with self.assertRaisesRegex(ValueError,'^native Docker command evidence already exists$'):self.run_command(root,code)
            self.assertEqual((root/'command-00.json').read_bytes(),original);self.assertFalse(marker.exists())

if __name__=='__main__':unittest.main()
