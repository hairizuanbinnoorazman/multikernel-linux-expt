#!/usr/bin/env python3
"""Exercise the actual profile argument checks before any privileged effects."""
import ast
from pathlib import Path
import types
import unittest

SOURCE=Path(__file__).with_name('test-runtime-docker-profile-live.py')


def argument_program(text):
    nodes=ast.parse(text).body
    start=next(i for i,node in enumerate(nodes) if isinstance(node,ast.Assert)
               and 'usage: SCRIPT' in ast.unparse(node))
    end=next(i for i,node in enumerate(nodes[start:],start) if isinstance(node,ast.Assign)
             and any(isinstance(target,ast.Name) and target.id=='work' for target in node.targets))
    return compile(ast.Module(body=nodes[start:end],type_ignores=[]),str(SOURCE),'exec')


def parse(flags):
    scope={'sys':types.SimpleNamespace(argv=['profile','/protected/work','/protected/source',*flags])}
    exec(argument_program(SOURCE.read_text()),scope)
    return scope


class ProfileArguments(unittest.TestCase):
    def test_full_parent_aggregate_invocation_is_accepted(self):
        result=parse(['--storage-pressure','--harden-services','--engine-observation','--aggregate-docker-health'])
        self.assertTrue(result['aggregate_docker_health'] and result['engine_observation'] and result['storage_pressure'] and result['harden_services'])

    def test_aggregate_requires_engine_and_ordinary_pressure(self):
        for flags in [['--aggregate-docker-health'],['--engine-observation','--aggregate-docker-health'],
                      ['--storage-record-loss','--engine-observation','--aggregate-docker-health'],
                      ['--storage-export-crash','--engine-observation','--aggregate-docker-health']]:
            with self.subTest(flags=flags),self.assertRaises(AssertionError):parse(flags)

    def test_duplicate_unknown_and_conflicting_scenarios_still_refuse(self):
        for flags in [['--storage-pressure','--storage-pressure'],['--unknown'],
                      ['--concurrent','--crash-runtime'],['--storage-pressure','--crash-during-io']]:
            with self.subTest(flags=flags),self.assertRaises(AssertionError):parse(flags)

    def test_existing_ordinary_and_engine_scenarios_still_accept(self):
        for flags in [[],['--storage-pressure'],['--storage-pressure','--harden-services','--engine-observation'],['--concurrent','--harden-services'],['--crash-during-io','--harden-services']]:
            with self.subTest(flags=flags):parse(flags)


if __name__=='__main__':unittest.main()
