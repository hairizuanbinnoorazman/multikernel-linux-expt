#!/usr/bin/env python3
"""Module-origin additions must not become a blanket taint exemption."""
import unittest
from runtime_module_taint import expected_module_taint


class ModuleTaintTests(unittest.TestCase):
    def test_exact_candidate_taint_includes_permitted_origin_addition(self):
        self.assertEqual(expected_module_taint(8192, 12288), 12288)
        self.assertEqual(expected_module_taint(8192, 8192), 8192)
        self.assertEqual(expected_module_taint(0, 12288), 12288)

    def test_existing_bits_are_preserved_without_excusing_new_warning(self):
        self.assertEqual(expected_module_taint(8193, 12289), 12289)
        for bit in range(64):
            if bit in (12, 13):
                continue
            with self.subTest(bit=bit), self.assertRaisesRegex(ValueError, '^unexpected taint'):
                expected_module_taint(8192, 12288 | (1 << bit))

    def test_cold_reset_is_not_a_same_boot_transition(self):
        with self.assertRaisesRegex(ValueError, '^module taint bits cleared'):
            expected_module_taint(12288, 8192)

    def test_invalid_values_never_echo_input(self):
        for value in [True, False, -1, 1 << 64, None, 'SYNTHETIC_PRIVATE_TAINT', 8192.0]:
            for before, after in [(value, 12288), (8192, value)]:
                with self.subTest(value=value), self.assertRaisesRegex(ValueError, '^invalid module taint observation$'):
                    expected_module_taint(before, after)


if __name__ == '__main__':
    unittest.main()
