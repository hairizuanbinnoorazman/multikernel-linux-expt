#!/usr/bin/env python3
"""Validate runtime contract schemas and their positive/negative fixtures."""

import json
import pathlib
import sys

import jsonschema


REPO = pathlib.Path(__file__).resolve().parent.parent
SCHEMAS = REPO / "docs/runtime/contracts/schemas"
FIXTURES = REPO / "docs/runtime/contracts/fixtures"


def strict_json(path: pathlib.Path):
    def object_pairs(pairs):
        result = {}
        for name, value in pairs:
            if name in result:
                raise ValueError(f"duplicate JSON object name {name!r}")
            result[name] = value
        return result

    with path.open(encoding="utf-8") as source:
        return json.load(source, object_pairs_hook=object_pairs)


def main() -> int:
    schemas = {}
    for path in sorted(SCHEMAS.glob("*.schema.json")):
        schema = strict_json(path)
        jsonschema.Draft202012Validator.check_schema(schema)
        schemas[path.name] = schema
    store = {schema["$id"]: schema for schema in schemas.values()}

    cases = strict_json(FIXTURES / "schema-cases.json")
    failures = []
    for case in cases:
        fixture = FIXTURES / case["fixture"]
        try:
            instance = strict_json(fixture)
            validator = jsonschema.Draft202012Validator(
                schemas[case["schema"]],
                resolver=jsonschema.RefResolver.from_schema(
                    schemas[case["schema"]], store=store
                ),
                format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER,
            )
            errors = sorted(validator.iter_errors(instance), key=lambda error: list(error.path))
            accepted = not errors
            detail = errors[0].message if errors else "accepted"
        except (json.JSONDecodeError, ValueError) as error:
            accepted = False
            detail = str(error)
        if accepted != case["valid"]:
            failures.append(f"{case['fixture']}: {detail}")

    if failures:
        print("runtime schema fixtures: FAIL", file=sys.stderr)
        for failure in failures:
            print(f"  {failure}", file=sys.stderr)
        return 1
    print(f"runtime schema fixtures: PASS ({len(schemas)} schemas, {len(cases)} cases)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
