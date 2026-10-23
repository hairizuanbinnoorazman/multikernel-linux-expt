#!/usr/bin/env python3
import json
import unittest
from runtime_storage_pressure import full_observation, recovered_observation


class StoragePressureTests(unittest.TestCase):
    full = ('G8_STORAGE_PRESSURE_BEFORE_KIB=1500000\n'
            'G8_STORAGE_PRESSURE_FS_MAGIC=ef53\n'
            'G8_STORAGE_PRESSURE_WRITE_EXIT=1\n'
            'G8_STORAGE_PRESSURE_SYNC_EXIT=0\n'
            'G8_STORAGE_PRESSURE_FULL_KIB=0\n'
            'G8_STORAGE_PRESSURE_FILE_BYTES=1500000000\n'
            'G8_STORAGE_PRESSURE_FULL\n')
    error = 'dd: error writing output: No space left on device\n'
    engine = {'ID': 'guest-engine', 'KernelVersion': 'approved-child',
              'ServerVersion': '29.9.0', 'Driver': 'vfs',
              'SecurityOptions': ['name=rootless'],
              'DockerRootDir': '/home/rootless/.local/share/docker'}

    def output(self, second=None):
        return (json.dumps(self.engine) + '\n' + self.full +
                'G8_STORAGE_PRESSURE_RESTORED_KIB=1499900\n' +
                json.dumps(self.engine if second is None else second) + '\n' +
                'G8_STORAGE_PRESSURE_BUILD_RECOVERED\nG8_BUILD_WORKLOAD_PASS\n')

    def test_exact_full_condition_and_recovered_engine(self):
        full = full_observation(self.full, self.error)
        self.assertEqual(full['full_available_kib'], 0)
        self.assertEqual(full['pressure_file_bytes'], 1500000000)
        recovered = recovered_observation(self.output(), self.error)
        self.assertEqual(recovered['engine_id'], 'guest-engine')
        self.assertEqual(recovered['restored_available_kib'], 1499900)

    def test_wrong_failure_or_ambiguous_full_measurements_refused(self):
        for old, new in [('WRITE_EXIT=1', 'WRITE_EXIT=0'), ('WRITE_EXIT=1', 'WRITE_EXIT=2'),
                         ('SYNC_EXIT=0', 'SYNC_EXIT=1'),
                         ('FULL_KIB=0', 'FULL_KIB=1'), ('FS_MAGIC=ef53', 'FS_MAGIC=1021994'),
                         ('FILE_BYTES=1500000000', 'FILE_BYTES=3000000000'),
                         ('FILE_BYTES=1500000000', 'FILE_BYTES=1024'),
                         ('BEFORE_KIB=1500000', 'BEFORE_KIB=10')]:
            with self.subTest(new=new), self.assertRaises(ValueError):
                full_observation(self.full.replace(old, new), self.error)
        with self.assertRaises(ValueError):
            full_observation(self.full, 'Input/output error')
        with self.assertRaises(ValueError):
            full_observation(self.full + 'G8_STORAGE_PRESSURE_FULL_KIB=0\n', self.error)
        with self.assertRaises(ValueError):
            full_observation(self.full + 'G8_STORAGE_PRESSURE_FULL_KIB=invalid\n', self.error)
        with self.assertRaises(ValueError):
            full_observation(self.full + 'G8_STORAGE_PRESSURE_FS_MAGIC=1021994\n', self.error)

    def test_space_loss_engine_change_or_missing_matrix_refused(self):
        with self.assertRaises(ValueError):
            recovered_observation(self.output().replace('RESTORED_KIB=1499900', 'RESTORED_KIB=100'), self.error)
        for key, value in [('ID', 'different'), ('DockerRootDir', '/host/docker'),
                           ('SecurityOptions', []), ('KernelVersion', 'primary')]:
            with self.subTest(key=key), self.assertRaises(ValueError):
                recovered_observation(self.output({**self.engine, key: value}), self.error)
        with self.assertRaises(ValueError):
            recovered_observation(self.output().replace('G8_BUILD_WORKLOAD_PASS', 'FAILED'), self.error)


if __name__ == '__main__':
    unittest.main()
