#!/usr/bin/env python3
"""Negative selection controls plus opt-in real pinned artifacts on GCE."""
import os
from pathlib import Path
import shutil
import tempfile
import unittest
from runtime_trial_helper import verify,PINS

class HelperSelectionTests(unittest.TestCase):
    def test_unknown_variant_and_unprotected_inputs_refused(self):
        with self.assertRaises(ValueError):verify('/not-a-file','arbitrary-hash')
        for value in ('relative','/tmp/../tmp/file'):
            with self.assertRaises(ValueError):verify(value)
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);file=root/'helper';file.write_bytes(b'unapproved bytes');file.chmod(0o500)
            with self.assertRaises(ValueError):verify(file,'server-queue-candidate')
            link=root/'link';link.symlink_to(file)
            with self.assertRaises((OSError,ValueError)):verify(link)

    def test_callers_bind_selection_before_mutation_and_forward_the_variant(self):
        root=Path(__file__).resolve().parent
        parent=(root/'test-runtime-vsock-module-trial.py').read_text()
        child=(root/'test-runtime-docker-profile-live.py').read_text()
        self.assertLess(parent.index('helper_selection=verify_trial_helper'),parent.index("run(['/sbin/rmmod'"))
        self.assertIn("'--setenv=MK_TRIAL_HELPER_VARIANT='+helper_selection['helper_variant']",parent)
        self.assertLess(child.index('helper_selection=verify_trial_helper'),child.index("private.mkdir("))
        self.assertIn("os.environ.get('MK_TRIAL_HELPER_VARIANT','qualified')",child)
        self.assertEqual(set(PINS),{'qualified','server-queue-candidate'})

    def test_real_root_owned_pinned_artifacts_and_corruptions(self):
        source=os.environ.get('MK_G8_TRIAL_HELPER_CANDIDATE')
        qualified=os.environ.get('MK_G8_TRIAL_HELPER_QUALIFIED')
        if not source or not qualified:self.skipTest('explicit GCE pinned artifacts required')
        self.assertEqual(os.geteuid(),0)
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);file=root/'helper'
            for name,path in [('qualified',qualified),('server-queue-candidate',source)]:
                shutil.copyfile(path,file);file.chmod(0o500)
                report=verify(file,name);self.assertEqual(report['helper_sha256'],PINS[name])
                self.assertEqual(report['candidate_selected'],name!='qualified')
                self.assertFalse(report['production_default_changed'])
                other='qualified' if name!='qualified' else 'server-queue-candidate'
                with self.assertRaises(ValueError):verify(file,other)
            original=file.read_bytes()
            for mode in ('mutated','writable','hardlink','nonroot','empty','directory-writable'):
                with self.subTest(mode=mode):
                    file.write_bytes(original);file.chmod(0o500)
                    if mode=='mutated':file.write_bytes(original[:-1]+bytes([original[-1]^1]))
                    if mode=='writable':file.chmod(0o522)
                    if mode=='hardlink':os.link(file,root/'alias')
                    if mode=='nonroot':os.chown(file,65534,65534)
                    if mode=='empty':file.write_bytes(b'')
                    if mode=='directory-writable':root.chmod(0o722)
                    with self.assertRaises(ValueError):verify(file,'server-queue-candidate')
                    root.chmod(0o700);os.chown(file,0,0)
                    if mode=='hardlink':(root/'alias').unlink()
            file.unlink();os.mkfifo(file)
            with self.assertRaises(ValueError):verify(file,'server-queue-candidate')
            file.unlink()
        print('REAL_PINNED_TRIAL_HELPER_BOTH_VARIANTS_AND_UNSAFE_ARTIFACT_REFUSALS_PASS')

if __name__=='__main__':unittest.main()
