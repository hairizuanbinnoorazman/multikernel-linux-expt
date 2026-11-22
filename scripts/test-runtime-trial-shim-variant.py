#!/usr/bin/env python3
import unittest
from runtime_trial_shim_variant import variant, require_production, PRODUCTION, EXHAUSTION


class VariantTests(unittest.TestCase):
    def test_historical_and_declared_production(self):
        for build in [{}, dict(shim_variant=PRODUCTION, instrumented_test_executable=False)]:
            self.assertEqual(variant(build), PRODUCTION)
            require_production(build)

    def test_instrumented_packet_has_no_ordinary_execution_authority(self):
        build = dict(shim_variant=EXHAUSTION, instrumented_test_executable=True)
        self.assertEqual(variant(build), EXHAUSTION)
        with self.assertRaisesRegex(ValueError, 'separate trial admission'):
            require_production(build)

    def test_unknown_partial_and_inconsistent_metadata_refused(self):
        cases = [None, [], dict(shim_variant=EXHAUSTION), dict(instrumented_test_executable=True)]
        for name in ['unknown', EXHAUSTION, PRODUCTION, False, None]:
            for instrumented in [None, 0, 1, 'true', name == PRODUCTION]:
                cases.append(dict(shim_variant=name, instrumented_test_executable=instrumented))
        for build in cases:
            with self.subTest(build=build), self.assertRaises(ValueError):
                variant(build)


if __name__ == '__main__':
    unittest.main()
