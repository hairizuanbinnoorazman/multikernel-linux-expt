#!/usr/bin/env python3
import unittest
from runtime_fixture_definitions import before_assignment


class FixtureDefinitionTests(unittest.TestCase):
    def test_quoted_marker_and_nested_assignment_do_not_truncate_checks(self):
        source = '''marker = "image=json.loads((w/"
# image = do_not_execute()
checks = []
def read_only():
    image = "nested assignment"
    return image
checks.append(read_only())
image = checks.append("phase entered")
checks.append("phase replayed")
'''
        namespace = {}
        prefix = before_assignment(source, 'image')
        exec(compile(prefix, '<retained-definitions>', 'exec'), namespace)
        self.assertEqual(namespace['checks'], ['nested assignment'])
        self.assertNotIn('image', namespace)
        self.assertNotIn('phase entered', prefix)
        self.assertNotIn('phase replayed', prefix)

    def test_multiline_definition_and_crlf_remain_compilable(self):
        source = 'value = (\r\n  42\r\n)\r\nimage = None\r\nraise RuntimeError("replayed")\r\n'
        namespace = {}
        exec(before_assignment(source, 'image'), namespace)
        self.assertEqual(namespace['value'], 42)

    def test_missing_duplicate_nested_and_compound_boundaries_refuse(self):
        for source in ['value=1\n', 'image=1\nimage=2\n',
                       'def f():\n image=1\n', 'value=1; image=2\n']:
            with self.subTest(source=source), self.assertRaisesRegex(ValueError, '^fixture boundary must be one top-level assignment$'):
                before_assignment(source, 'image')

    def test_malformed_source_and_names_never_echo_input(self):
        for source in ['SYNTHETIC_PRIVATE_FIXTURE = "unterminated', 'x' * ((1 << 20) + 1), None]:
            with self.assertRaisesRegex(ValueError, '^invalid retained fixture source$'):
                before_assignment(source, 'image')
        for name in ['SYNTHETIC_PRIVATE_FIXTURE invalid', 'if', None]:
            with self.assertRaisesRegex(ValueError, '^invalid fixture boundary name$'):
                before_assignment('image=1\n', name)


if __name__ == '__main__':
    unittest.main()
