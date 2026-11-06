#!/usr/bin/env python3
import unittest
from runtime_fixture_epoch import qualify_unit_epoch, unit_properties

OLD = '9c7a1110-08a1-41b2-a0af-7d214af1ded3'
NEW = '6349e148-e008-4fa9-a36c-83549cfe2501'
INVOCATION = 'f975d9ec10e1409bbd513ac08da423e8'


def completed():
    return dict(MainPID='0', InvocationID=INVOCATION, SubState='exited',
                Result='success', ExecMainStatus='0', ControlGroup='')


class FixtureEpochTests(unittest.TestCase):
    def test_same_boot_exact_completed_unit_is_status_evidence_only(self):
        value = qualify_unit_epoch(OLD, INVOCATION, OLD, completed())
        self.assertEqual(value, dict(same_boot=True, invocation_matches=True,
                                    unit_completion_qualified=True, code='SAME_BOOT_COMPLETED_UNIT'))
        self.assertNotIn('normal_cleanup_qualified', value)
        self.assertNotIn('recovery_qualified', value)

    def test_reboot_never_relabels_old_success_as_current_completion(self):
        value = qualify_unit_epoch(OLD, INVOCATION, NEW, completed())
        self.assertFalse(value['same_boot'])
        self.assertFalse(value['unit_completion_qualified'])
        self.assertEqual(value['code'], 'BOOT_CHANGED')

    def test_absent_and_recreated_units_do_not_inherit_history(self):
        for boot in [OLD, NEW]:
            for invocation in ['', 'b' * 32]:
                unit = completed()
                unit.update(InvocationID=invocation, SubState='dead')
                value = qualify_unit_epoch(OLD, INVOCATION, boot, unit)
                self.assertFalse(value['unit_completion_qualified'])

    def test_failed_running_or_nonzero_status_cannot_pass(self):
        for key, value in [('MainPID', '9'), ('SubState', 'running'), ('Result', 'exit-code'), ('ExecMainStatus', '1')]:
            unit = completed()
            unit[key] = value
            result = qualify_unit_epoch(OLD, INVOCATION, OLD, unit)
            self.assertFalse(result['unit_completion_qualified'])
            self.assertEqual(result['code'], 'UNIT_NOT_SUCCESSFULLY_EXITED')

    def test_parser_refuses_duplicate_missing_case_alias_and_private_input(self):
        text = ''.join(k + '=' + v + '\n' for k, v in completed().items())
        self.assertEqual(unit_properties(text), completed())
        for invalid in [text + 'MainPID=0\n', text.replace('MainPID', 'MainPid'),
                        text.replace('Result=success\n', ''), text + 'SYNTHETIC_PRIVATE_TOKEN=value\n',
                        text.replace('MainPID=0', 'MainPID=00'), text.replace('ExecMainStatus=0', 'ExecMainStatus=256')]:
            with self.subTest(invalid=invalid), self.assertRaisesRegex(ValueError, '^invalid fixture unit observation$'):
                unit_properties(invalid)

    def test_bad_epoch_values_refuse_without_echo(self):
        for value in [None, True, OLD.upper(), 'SYNTHETIC_PRIVATE_TOKEN']:
            with self.assertRaisesRegex(ValueError, '^invalid fixture epoch identity$'):
                qualify_unit_epoch(value, INVOCATION, NEW, completed())
        unit = completed()
        unit['MainPID'] = False
        with self.assertRaisesRegex(ValueError, '^invalid fixture unit observation$'):
            qualify_unit_epoch(OLD, INVOCATION, NEW, unit)


if __name__ == '__main__':
    unittest.main()
