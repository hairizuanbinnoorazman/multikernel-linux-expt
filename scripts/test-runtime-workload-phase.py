#!/usr/bin/env python3
import json,pathlib,subprocess,tempfile,time,unittest
from runtime_workload_phase import wait_for_phase,PhaseFailure

class PhaseTests(unittest.TestCase):
    def exercise(self, body, out=b'', err=b'', duration=.05):
        with tempfile.TemporaryDirectory() as raw:
            d=pathlib.Path(raw);stdout=d/'stdout';stderr=d/'stderr';checkpoint=d/'checkpoint.json'
            stdout.write_bytes(out);stderr.write_bytes(err)
            p=subprocess.Popen(['/bin/sh','-c',body],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            try:
                try:wait_for_phase(p,stdout,stderr,[b'FULL\n'],[b'No space left'],time.monotonic()+duration,checkpoint,'invocation','boot','enospc')
                except PhaseFailure:pass
                return json.loads(checkpoint.read_text())
            finally:
                if p.poll() is None:p.terminate()
                p.wait(timeout=2)
    def test_live_markers_reach_phase(self):
        r=self.exercise('exec sleep 5',b'FULL\n',b'No space left');self.assertEqual(r['outcome'],'reached');self.assertIsNone(r['process_returncode']);self.assertFalse(r['cleanup_qualified'])
    def test_exit_and_deadline_are_distinct(self):
        r=self.exercise('exit 37',duration=1);self.assertEqual(r['outcome'],'process-exited');self.assertEqual(r['process_returncode'],37)
        r=self.exercise('exec sleep 5');self.assertEqual(r['outcome'],'deadline-exceeded');self.assertIsNone(r['process_returncode'])
    def test_output_limit_is_bounded_and_does_not_export_contents(self):
        r=self.exercise('exec sleep 5',b'synthetic-secret'*20000);self.assertEqual(r['outcome'],'output-limit');self.assertEqual(r['output']['stdout']['sample_bytes'],131073);self.assertNotIn('synthetic-secret',json.dumps(r))
    def test_expired_deadline_does_not_accept_markers(self):
        self.assertEqual(self.exercise('exec sleep 5',b'FULL\n',b'No space left',duration=-1)['outcome'],'deadline-exceeded')

if __name__=='__main__':unittest.main()
