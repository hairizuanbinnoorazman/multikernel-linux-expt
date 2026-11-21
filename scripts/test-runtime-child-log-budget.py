#!/usr/bin/env python3
"""Actual shared-deadline/late-completion and private native capture controls."""
from pathlib import Path
import json,subprocess,sys,tempfile,time,unittest
from unittest.mock import patch
import runtime_child_log_pressure as subject
from runtime_trial_command import capture


class Clock:
    def __init__(self):self.ns=0;self.sleeps=[]
    def now(self):return self.ns
    def sleep(self,seconds):self.sleeps.append(seconds);self.ns+=int(seconds*1e9)


class BudgetTests(unittest.TestCase):
    def test_late_success_failure_and_exact_deadline_all_refuse(self):
        for status,finish in [(0,301),(0,300),(1,300),(1,301)]:
            clock=Clock();calls=[]
            def probe(attempt,deadline):
                calls.append(deadline);clock.ns=finish*1_000_000_000
                return subprocess.CompletedProcess([],status)
            with self.subTest(status=status,finish=finish),patch.object(subject.time,'monotonic_ns',clock.now),patch.object(subject.time,'sleep',clock.sleep):
                with self.assertRaisesRegex(ValueError,'^child log writer did not complete within deadline$'):subject.wait_for_writer(probe,300_000_000_000)
            self.assertEqual(len(calls),1);self.assertEqual(clock.sleeps,[])

    def test_clips_each_probe_to_original_budget_and_keeps_ordinary_cap(self):
        clock=Clock();deadlines=[]
        def probe(attempt,deadline):
            deadlines.append(deadline);clock.ns+=25_000_000_000 if attempt<2 else 1_000_000_000
            return subprocess.CompletedProcess([],int(attempt<2))
        with patch.object(subject.time,'monotonic_ns',clock.now),patch.object(subject.time,'sleep',clock.sleep):subject.wait_for_writer(probe,60_000_000_000)
        self.assertEqual(deadlines,[30_000_000_000,56_000_000_000,60_000_000_000])

    def test_expired_and_sleep_exhausted_budget_make_no_further_probe(self):
        clock=Clock();clock.ns=1_000_000_000;calls=[]
        def probe(attempt,deadline):calls.append(attempt);return subprocess.CompletedProcess([],1)
        with patch.object(subject.time,'monotonic_ns',clock.now),patch.object(subject.time,'sleep',clock.sleep):
            with self.assertRaises(ValueError):subject.wait_for_writer(probe,clock.ns)
            self.assertEqual(calls,[])
            with self.assertRaises(ValueError):subject.wait_for_writer(probe,clock.ns+250_000_000)
        self.assertEqual(calls,[0]);self.assertEqual(clock.sleeps,[.25])

    def test_native_capture_receives_remaining_budget_and_retains_timeout_privately(self):
        with tempfile.TemporaryDirectory() as raw:
            private=Path(raw);deadline=time.monotonic_ns()+250_000_000
            def probe(attempt,limit):
                return capture([sys.executable,'-c',"import sys,time;sys.stderr.write('PRIVATE_READINESS_CANARY');sys.stderr.flush();time.sleep(.6)"],private,'log-ready-'+str(attempt),False,limit)
            with self.assertRaises(subprocess.TimeoutExpired) as refused:subject.wait_for_writer(probe,deadline)
            self.assertEqual(refused.exception.cmd,['owned-trial-command'])
            record=json.loads((private/'log-ready-0-timeout.json').read_text())
            self.assertEqual(record['deadline_monotonic_ns'],deadline);self.assertIs(record['qualified'],False)
            self.assertFalse((private/'log-ready-0.exit.json').exists());self.assertFalse((private/'log-ready-1.stdout').exists())

    def test_native_success_keeps_original_capture_and_qualifies_before_deadline(self):
        with tempfile.TemporaryDirectory() as raw:
            private=Path(raw);deadline=time.monotonic_ns()+2_000_000_000
            subject.wait_for_writer(lambda attempt,limit:capture([sys.executable,'-c','pass'],private,'log-ready-'+str(attempt),False,limit),deadline)
            self.assertEqual(json.loads((private/'log-ready-0.exit.json').read_text()),dict(returncode=0))
            self.assertFalse((private/'log-ready-0-timeout.json').exists())

if __name__=='__main__':unittest.main()
