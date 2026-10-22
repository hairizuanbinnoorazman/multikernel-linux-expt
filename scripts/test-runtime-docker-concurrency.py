#!/usr/bin/env python3
import unittest
from runtime_docker_concurrency import run_markers


class RunMarkerTests(unittest.TestCase):
    def test_only_actual_run_output_counts(self):
        echoed = '#5 [build 2/2] RUN ["/bin/busybox", "sh", "-ec", "echo G8_OVERLAP_RUN_BEGIN; echo G8_OVERLAP_RUN_END"]\n'
        self.assertEqual(run_markers(echoed), set())
        self.assertEqual(run_markers(echoed + '#5 0.123 G8_OVERLAP_RUN_BEGIN\n'), {'BEGIN'})
        self.assertEqual(run_markers(echoed + '#5 0.123 G8_OVERLAP_RUN_BEGIN\n#5 45.125 G8_OVERLAP_RUN_END\n'), {'BEGIN', 'END'})

    def test_dockerfile_diagnostics_and_cached_steps_do_not_prove_execution(self):
        text = '3 | RUN echo G8_OVERLAP_RUN_BEGIN\n#5 CACHED\nG8_OVERLAP_RUN_BEGIN\n'
        self.assertEqual(run_markers(text), set())


if __name__ == '__main__':
    unittest.main()
