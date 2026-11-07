#!/usr/bin/env python3
from pathlib import Path
import unittest
from runtime_trial_layout import stage_path,validate_stage

class LayoutTests(unittest.TestCase):
    def test_storage_default_is_separate_from_image_directory(self):
        p=stage_path('abcdef0');self.assertEqual(p,Path('/srv/multikernel-storage/stages/g8-host-abcdef0'))
        self.assertNotEqual(p,Path('/srv/multikernel-storage')/p.name)
        self.assertEqual(validate_stage(p,'abcdef0'),p)
    def test_legacy_layout_stays_explicit_and_inspectable(self):
        p=stage_path('abcdef0','root');self.assertEqual(p,Path('/opt/mkruntime/g8-host-abcdef0'));self.assertEqual(validate_stage(p),p)
    def test_unknown_root_traversal_alias_prefix_or_revision_refuses(self):
        for p in ['/tmp/g8-host-abcdef0','/srv/multikernel-storage/g8-host-abcdef0','/opt/mkruntime/g8-host-abcdef0/../g8-host-1234567','/opt/mkruntime-evil/g8-host-abcdef0','relative','/opt/mkruntime/g8-host-private-canary']:
            with self.subTest(path=p),self.assertRaisesRegex(ValueError,'^invalid private trial layout$'):validate_stage(p)
        with self.assertRaises(ValueError):validate_stage(stage_path('abcdef0'),'1234567')
        for revision,location in [(True,'storage'),('ABCDEF0','storage'),('abcdef0','/tmp'),('abcdef0',None)]:
            with self.assertRaises(ValueError):stage_path(revision,location)

if __name__=='__main__':unittest.main()
