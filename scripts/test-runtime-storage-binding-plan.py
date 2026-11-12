#!/usr/bin/env python3
"""Compare read-only preparation against a real update in a disposable root."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


planner = module('binding_planner', 'plan-runtime-storage-binding.py')
fixtures = module('deployment_fixtures', 'test-manage-runtime-deployment.py')


def snapshot(root):
    result = {}
    for parent, dirs, files in os.walk(root, followlinks=False):
        for name in ['.', *dirs, *files]:
            path = Path(parent) / name
            info = path.lstat()
            result[str(path.relative_to(root))] = (
                info.st_ino, info.st_mode, info.st_size, info.st_mtime_ns,
                info.st_ctime_ns, os.readlink(path) if path.is_symlink()
                else path.read_bytes() if path.is_file() else None,
            )
    return result


class BindingPlanTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='binding-plan-')
        self.addCleanup(self.temporary.cleanup)
        self.work = Path(self.temporary.name)
        self.root = self.work / 'root'
        self.root.mkdir()
        env, host = fixtures.inputs(self.work, 'plan', 'ens4')
        fixtures.run(self.root, 'install', str(env), str(host))
        self.old = int(next(line.split('=', 1)[1] for line in env.read_text().splitlines()
                            if line.startswith('MKRUNTIME_STORAGE_BYTES=')))
        self.new = self.old + (8 << 30)

    def test_prediction_matches_real_clone_without_changing_source_root(self):
        before = snapshot(self.root)
        plan = planner.prepare(self.root, self.old, self.new)
        self.assertEqual(snapshot(self.root), before)
        self.assertEqual(fixtures.active(self.root), 'deployments/' + plan['previous'])
        self.assertFalse((self.root / 'etc/multikernel/deployments' / plan['candidate']).exists())
        self.assertEqual(plan['changed_files'], ['runtime.env'])
        replica = self.work / 'replica'
        shutil.copytree(self.root, replica, symlinks=True)
        actual = json.loads(fixtures.run(replica, 'update-storage-size', str(self.old), str(self.new)).stdout)
        self.assertEqual(actual['active'], plan['candidate'])
        self.assertEqual(actual['previous'], plan['previous'])
        self.assertEqual(actual['changed_files'], plan['changed_files'])
        directory = replica / 'etc/multikernel/deployments' / plan['candidate']
        for name, mode in plan['modes'].items():
            self.assertEqual((directory / name).stat().st_mode & 0o777, mode)
        self.assertEqual(snapshot(self.root), before)

    def test_stale_shrink_and_unaligned_authority_refuse_without_writes(self):
        before = snapshot(self.root)
        for old, new in [(self.old + 512, self.new), (self.old, self.old - 512),
                         (self.old, self.new + 1)]:
            with self.subTest(old=old, new=new), self.assertRaises(ValueError):
                planner.prepare(self.root, old, new)
            self.assertEqual(snapshot(self.root), before)

    def test_writable_selected_asset_refuses_without_repair(self):
        env = (self.root / 'etc/multikernel/runtime.env').resolve()
        env.chmod(0o622)
        before = snapshot(self.root)
        with self.assertRaises(ValueError):
            planner.prepare(self.root, self.old, self.new)
        self.assertEqual(snapshot(self.root), before)

    def test_manager_pin_is_checked_before_execution(self):
        marker = self.work / 'executed'
        wrong = self.work / 'manager.py'
        wrong.write_text('from pathlib import Path\nPath(' + repr(str(marker)) + ').touch()\n')
        with self.assertRaises(ValueError):
            planner.load_manager(wrong)
        self.assertFalse(marker.exists())

    def test_existing_candidate_refuses_without_reusing_or_repairing_it(self):
        plan = planner.prepare(self.root, self.old, self.new)
        candidate = self.root / 'etc/multikernel/deployments' / plan['candidate']
        candidate.symlink_to(self.work / 'absent')
        before = snapshot(self.root)
        with self.assertRaises(ValueError):
            planner.prepare(self.root, self.old, self.new)
        self.assertEqual(snapshot(self.root), before)

    def test_cli_exports_closed_flags_and_private_refusals(self):
        before = snapshot(self.root)
        command = [sys.executable, str(Path(planner.__file__)), '--root', str(self.root)]
        result = subprocess.run([*command, str(self.old), str(self.new)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        flags = json.loads(result.stdout)
        self.assertTrue(flags['only_runtime_env_change'] and flags['asset_modes_preserved'])
        self.assertFalse(flags['disk_resize_performed'] or flags['deployment_selected'] or flags['service_restart_performed'])
        self.assertNotIn(fixtures.active(self.root).split('/')[1], result.stdout)
        for args in [['private-not-a-size', str(self.new)], [str(self.old), str(self.new + 1)],
                     ['--private-unknown-option'], []]:
            refused = subprocess.run([*command, *args], capture_output=True, text=True)
            self.assertEqual(refused.returncode, 1)
            self.assertEqual(refused.stdout, '')
            self.assertEqual(refused.stderr, 'runtime storage binding plan refused\n')
        self.assertEqual(snapshot(self.root), before)


if __name__ == '__main__':
    unittest.main()
