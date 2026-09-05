#!/usr/bin/env python3
"""Validate every committed runtime evidence manifest and its file references."""

import json
import hashlib
from pathlib import Path
import sys

import jsonschema

REPO = Path(__file__).resolve().parent.parent
SCHEMA_PATH = REPO / "docs/runtime/contracts/schemas/evidence-manifest-v1.schema.json"
EXCEPTIONS_PATH = REPO / "evidence/runtime-manifest-exceptions.json"


def strict_json(path):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError(f"duplicate JSON object name {key!r}")
            result[key] = value
        return result
    with path.open(encoding="utf-8") as source:
        return json.load(source, object_pairs_hook=pairs)


def relative(path):
    return path.relative_to(REPO).as_posix()


def referenced_file_issues(path, manifest):
    issues = []
    references = []
    repository = manifest.get("repository", {})
    if repository.get("dirty") is True and not repository.get("diff_output"):
        issues.append("dirty-without-diff")
    elif repository.get("diff_output"):
        references.append(("repository.diff_output", repository["diff_output"]))
    for command in manifest.get("commands", []):
        if "output_path" in command:
            references.append((f"command:{command.get('id', '?')}", command["output_path"]))
            if command["output_path"].lower().endswith("readme.md"):
                issues.append("command-points-to-narrative")
    for assertion in manifest.get("assertions", []):
        for evidence_path in assertion.get("evidence_paths", []):
            references.append((f"assertion:{assertion.get('id', '?')}", evidence_path))
            if evidence_path.lower().endswith("readme.md"):
                issues.append("assertion-points-to-narrative")
    cleanup = manifest.get("cleanup", {})
    for key in ("resources_before", "resources_after"):
        if key in cleanup:
            references.append((f"cleanup.{key}", cleanup[key]))
            if not cleanup[key].lower().endswith(".json"):
                issues.append("cleanup-ledger-not-structured")
    for owner, name in references:
        target = path.parent / name
        try:
            target.resolve(strict=True).relative_to(path.parent.resolve())
        except (FileNotFoundError, ValueError):
            issues.append(f"missing-reference:{owner}:{name}")
    if manifest.get("result") == "pass" and any(not item.get("passed", False) for item in manifest.get("assertions", [])):
        issues.append("pass-has-failed-assertion")
    return sorted(set(issues))


def main():
    schema = strict_json(SCHEMA_PATH)
    validator = jsonschema.Draft202012Validator(schema, format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER)
    document = strict_json(EXCEPTIONS_PATH) if EXCEPTIONS_PATH.exists() else {"schema_version": 1, "manifests": {}}
    if document.get("schema_version") != 1 or not isinstance(document.get("manifests"), dict):
        print(f"invalid exception registry: {relative(EXCEPTIONS_PATH)}", file=sys.stderr)
        return 1
    exceptions = document["manifests"]
    paths = sorted(path for directory in REPO.glob("evidence/runtime-*") for path in directory.rglob("manifest*.json"))
    failures, classified, seen = [], 0, set()
    for path in paths:
        name = relative(path)
        seen.add(name)
        try:
            manifest = strict_json(path)
            schema_errors = sorted(validator.iter_errors(manifest), key=lambda error: list(error.absolute_path))
            issues = ["schema:" + error.json_path + ":" + error.validator for error in schema_errors]
            issues.extend(referenced_file_issues(path, manifest))
        except (OSError, ValueError, json.JSONDecodeError) as error:
            issues = ["parse-error:" + type(error).__name__]
        issues = sorted(set(issues))
        exception = exceptions.get(name)
        if issues:
            digest = hashlib.sha256(path.read_bytes()).hexdigest()
            valid_exception = (
                isinstance(exception, dict)
                and exception.get("manifest_sha256") == digest
                and isinstance(exception.get("classifications"), list)
                and exception["classifications"]
                and all(isinstance(item, str) and item for item in exception["classifications"])
                and isinstance(exception.get("explanation"), str)
                and exception["explanation"]
            )
            if not valid_exception:
                failures.append(f"{name}: {json.dumps(issues)}")
            else:
                classified += 1
        elif exception is not None:
            failures.append(f"{name}: stale exception for a conforming manifest")
    for unknown in sorted(set(exceptions) - seen):
        failures.append(f"exception names nonexistent manifest: {unknown}")
    if failures:
        print("runtime evidence manifests: FAIL", file=sys.stderr)
        for failure in failures:
            print("  " + failure, file=sys.stderr)
        return 1
    print(f"runtime evidence manifests: PASS ({len(paths)} checked, {classified} historical nonconforming and explicitly classified)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
