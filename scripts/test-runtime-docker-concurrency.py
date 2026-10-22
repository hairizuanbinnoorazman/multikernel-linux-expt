#!/usr/bin/env python3
import unittest
import hashlib
from runtime_docker_concurrency import run_markers, owner_for_container


class RunMarkerTests(unittest.TestCase):
    def test_only_actual_run_output_counts(self):
        echoed = '#5 [build 2/2] RUN ["/bin/busybox", "sh", "-ec", "echo G8_OVERLAP_RUN_BEGIN; echo G8_OVERLAP_RUN_END"]\n'
        self.assertEqual(run_markers(echoed), set())
        self.assertEqual(run_markers(echoed + '#5 0.123 G8_OVERLAP_RUN_BEGIN\n'), {'BEGIN'})
        self.assertEqual(run_markers(echoed + '#5 0.123 G8_OVERLAP_RUN_BEGIN\n#5 45.125 G8_OVERLAP_RUN_END\n'), {'BEGIN', 'END'})

    def test_owner_mapping_follows_container_identity_in_either_allocation_order(self):
        ids = ['a'*64, 'b'*64]
        owners = [{'sandbox_id': 'mk-' + value[:32] + '-' + hashlib.sha256(('moby\0' + value).encode()).hexdigest()[:16], 'child_cid': cid} for value, cid in zip(ids, [41,40])]
        self.assertEqual(owner_for_container(ids[0], list(reversed(owners)))['child_cid'], 41)
        self.assertEqual(owner_for_container(ids[1], owners)['child_cid'], 40)
        with self.assertRaises(ValueError):owner_for_container('c'*64, owners)
        with self.assertRaises(ValueError):owner_for_container(ids[0], owners+owners)

    def test_dockerfile_diagnostics_and_cached_steps_do_not_prove_execution(self):
        text = '3 | RUN echo G8_OVERLAP_RUN_BEGIN\n#5 CACHED\nG8_OVERLAP_RUN_BEGIN\n'
        self.assertEqual(run_markers(text), set())


if __name__ == '__main__':
    unittest.main()
