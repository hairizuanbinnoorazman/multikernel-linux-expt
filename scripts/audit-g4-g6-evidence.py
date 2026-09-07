#!/usr/bin/env python3
"""Close G4-G6 only when raw evidence satisfies the final-run contract."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import stat
import sys

import jsonschema


REPO = Path(__file__).resolve().parent.parent
MANIFEST_SCHEMA = REPO / "docs/runtime/contracts/schemas/evidence-manifest-v1.schema.json"
LEDGER_SCHEMA = REPO / "docs/runtime/contracts/schemas/gce-resource-ledger-v1.schema.json"
REQUIREMENTS = REPO / "docs/runtime/contracts/g4-g6-evidence-requirements-v1.json"
GATES = ("G4", "G5", "G6")


class AuditError(ValueError):
    pass


def strict_json(path):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise AuditError(f"{path}: duplicate JSON key {key!r}")
            result[key] = value
        return result

    try:
        with path.open(encoding="utf-8") as source:
            return json.load(source, object_pairs_hook=pairs)
    except json.JSONDecodeError as error:
        raise AuditError(f"{path}: invalid JSON: {error}") from error


def strict_json_line(value, label):
    def pairs(items):
        result = {}
        for key, item in items:
            if key in result:
                raise AuditError(f"{label}: duplicate JSON key {key!r}")
            result[key] = item
        return result

    try:
        return json.loads(value, object_pairs_hook=pairs)
    except json.JSONDecodeError as error:
        raise AuditError(f"{label}: invalid JSON capture envelope") from error


def validator(schema_path):
    schema = strict_json(schema_path)
    return jsonschema.Draft202012Validator(
        schema, format_checker=jsonschema.Draft202012Validator.FORMAT_CHECKER
    )


def schema_check(document, checked_by, label):
    errors = sorted(checked_by.iter_errors(document), key=lambda item: list(item.absolute_path))
    if errors:
        details = "; ".join(f"{item.json_path}: {item.message}" for item in errors)
        raise AuditError(f"{label}: schema validation failed: {details}")


def regular_file_within(bundle, name):
    candidate = bundle / name
    try:
        resolved = candidate.resolve(strict=True)
        resolved.relative_to(bundle)
        info = candidate.lstat()
    except (FileNotFoundError, ValueError) as error:
        raise AuditError(f"unsafe or missing bundle reference: {name}") from error
    if not stat.S_ISREG(info.st_mode) or candidate.is_symlink():
        raise AuditError(f"bundle reference is not a regular non-symlink file: {name}")
    return candidate


def transcript(path, expected_argv, expected_status):
    lines = path.read_text(encoding="utf-8").splitlines()
    if len(lines) < 2:
        raise AuditError(f"{path.name}: command transcript is incomplete")
    header = strict_json_line(lines[0], path.name)
    trailer = strict_json_line(lines[-1], path.name)
    command = header.get("evidence_command", {})
    result = trailer.get("evidence_command_result", {})
    if command.get("argv") != expected_argv:
        raise AuditError(f"{path.name}: captured argv differs from manifest")
    if result.get("exit_status") != expected_status:
        raise AuditError(f"{path.name}: captured exit status differs from manifest")
    for owner, value in (("started_at", command.get("started_at")), ("ended_at", result.get("ended_at"))):
        if not isinstance(value, str) or not value.endswith("Z"):
            raise AuditError(f"{path.name}: capture {owner} is missing or non-UTC")
    return "\n".join(lines[1:-1]) + "\n"


def load_requirements():
    document = strict_json(REQUIREMENTS)
    if document.get("schema_version") != 1:
        raise AuditError("unsupported G4-G6 requirements version")
    if set(document.get("required_gate_assertions", {})) != set(GATES):
        raise AuditError("requirements must define exactly G4, G5, and G6")
    components = document.get("required_components")
    if not isinstance(components, list) or not components or len(components) != len(set(components)):
        raise AuditError("requirements components must be a unique non-empty list")
    for gate, names in document["required_gate_assertions"].items():
        if not isinstance(names, list) or not names or len(names) != len(set(names)):
            raise AuditError(f"{gate} requirements must be a unique non-empty list")
    return document


def sha256(path):
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def validate_ledger(bundle, name, ledger_validator, project):
    path = regular_file_within(bundle, name)
    document = strict_json(path)
    schema_check(document, ledger_validator, name)
    if document["project"] != project:
        raise AuditError(f"{name}: project differs from manifest host")
    return document


def audit(bundle):
    bundle = bundle.resolve(strict=True)
    if not bundle.is_dir():
        raise AuditError("bundle must be a directory")
    requirements = load_requirements()
    manifest_validator = validator(MANIFEST_SCHEMA)
    ledger_validator = validator(LEDGER_SCHEMA)
    manifests = {}
    common_repository = None
    common_host = None
    common_components = None
    common_ledgers = None

    for gate in GATES:
        name = f"manifest-{gate.lower()}.json"
        manifest = strict_json(regular_file_within(bundle, name))
        schema_check(manifest, manifest_validator, name)
        if manifest["gate"] != gate:
            raise AuditError(f"{name}: gate must be {gate}")
        if manifest["result"] != "pass":
            raise AuditError(f"{name}: final gate result must be pass")
        if manifest["limitations"]:
            raise AuditError(f"{name}: a final pass may not retain limitations")
        if manifest["repository"]["dirty"] and not manifest["repository"].get("diff_output"):
            raise AuditError(f"{name}: dirty source requires an exact diff artifact")
        if manifest["repository"].get("diff_output"):
            regular_file_within(bundle, manifest["repository"]["diff_output"])
        if manifest["host"]["provider"] != "gce" or not all(
            manifest["host"].get(key) for key in ("project", "zone", "instance")
        ):
            raise AuditError(f"{name}: final evidence requires a fully identified GCE host")

        command_ids = [item["id"] for item in manifest["commands"]]
        assertion_ids = [item["id"] for item in manifest["assertions"]]
        if len(command_ids) != len(set(command_ids)):
            raise AuditError(f"{name}: duplicate command IDs")
        if len(assertion_ids) != len(set(assertion_ids)):
            raise AuditError(f"{name}: duplicate assertion IDs")

        outputs = {}
        for command in manifest["commands"]:
            path = regular_file_within(bundle, command["output_path"])
            outputs[command["output_path"]] = transcript(
                path, command["argv"], command["exit_status"]
            )
        required = set(requirements["required_gate_assertions"][gate])
        missing = sorted(required - set(assertion_ids))
        if missing:
            raise AuditError(f"{name}: missing required assertions: {', '.join(missing)}")
        for assertion in manifest["assertions"]:
            if not assertion["passed"]:
                raise AuditError(f"{name}: failed assertion {assertion['id']}")
            raw_paths = set(assertion["evidence_paths"]) & set(outputs)
            if not raw_paths:
                raise AuditError(
                    f"{name}: assertion {assertion['id']} has no raw command transcript"
                )
            marker = f"EVIDENCE_ASSERTION id={assertion['id']} passed=true"
            if not any(marker in outputs[path].splitlines() for path in raw_paths):
                raise AuditError(
                    f"{name}: assertion {assertion['id']} lacks its raw PASS marker"
                )
            for evidence_path in assertion["evidence_paths"]:
                if evidence_path.lower().endswith(("readme", "readme.md")):
                    raise AuditError(f"{name}: narrative file used as assertion evidence")
                regular_file_within(bundle, evidence_path)

        cleanup = manifest["cleanup"]
        if not cleanup["completed"] or not cleanup["operator_acknowledged"]:
            raise AuditError(f"{name}: cleanup is not completed and acknowledged")
        before = validate_ledger(
            bundle, cleanup["resources_before"], ledger_validator, manifest["host"]["project"]
        )
        after = validate_ledger(
            bundle, cleanup["resources_after"], ledger_validator, manifest["host"]["project"]
        )
        ledger_identity = (
            json.dumps(before, sort_keys=True),
            json.dumps(after, sort_keys=True),
        )
        instance = manifest["host"]["instance"]
        before_instances = [item for item in before["instances"] if item["name"] == instance]
        if len(before_instances) != 1:
            raise AuditError(f"{name}: before ledger lacks tested instance")
        if any(item["name"] == instance for item in after["instances"]):
            raise AuditError(f"{name}: after ledger still contains tested instance")
        attachments = before_instances[0]["disks"]
        boot_disks = [item for item in attachments if item["boot"]]
        retained_disks = [item for item in attachments if not item["boot"]]
        if len(boot_disks) != 1 or not boot_disks[0]["auto_delete"]:
            raise AuditError(f"{name}: tested boot disk must be uniquely auto-deleted")
        if not retained_disks or any(item["auto_delete"] for item in retained_disks):
            raise AuditError(f"{name}: retained data disks must have auto-delete disabled")
        after_disk_names = {item["name"] for item in after["disks"]}
        if boot_disks[0]["name"] in after_disk_names:
            raise AuditError(f"{name}: auto-delete boot disk remains after cleanup")
        if not all(item["name"] in after_disk_names for item in retained_disks):
            raise AuditError(f"{name}: retained data disk is missing after cleanup")
        if not cleanup["retained_resources"]:
            raise AuditError(f"{name}: retained resources must be explicitly named")

        component_map = {
            item["name"]: (item["version"], item["sha256"])
            for item in manifest["components"]
        }
        missing_components = sorted(set(requirements["required_components"]) - set(component_map))
        if missing_components:
            raise AuditError(f"{name}: missing components: {', '.join(missing_components)}")
        repository_identity = json.dumps(manifest["repository"], sort_keys=True)
        host_identity = json.dumps(manifest["host"], sort_keys=True)
        selected_components = {key: component_map[key] for key in requirements["required_components"]}
        if common_repository is None:
            common_repository = repository_identity
            common_host = host_identity
            common_components = selected_components
            common_ledgers = ledger_identity
        elif (repository_identity, host_identity, selected_components) != (
            common_repository, common_host, common_components
        ):
            raise AuditError(f"{name}: source, host, or installed component identity differs")
        elif ledger_identity != common_ledgers:
            raise AuditError(f"{name}: before/after cloud ledgers differ across gates")
        manifests[gate] = manifest

    return {
        "schema_version": 1,
        "algorithm": "sha256",
        "audited_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "requirements_sha256": sha256(REQUIREMENTS),
        "gates": list(GATES),
        "files": [],
    }


def inventory(bundle, excluded):
    result = []
    for path in sorted(bundle.rglob("*")):
        if path == excluded:
            continue
        if path.is_symlink():
            raise AuditError(f"bundle contains a symlink: {path.relative_to(bundle)}")
        if not path.is_file():
            if not path.is_dir():
                raise AuditError(f"bundle contains unsupported entry: {path.relative_to(bundle)}")
            continue
        result.append(
            {
                "path": path.relative_to(bundle).as_posix(),
                "size_bytes": path.stat().st_size,
                "sha256": sha256(path),
            }
        )
    return result


def write_index(bundle, output):
    bundle = bundle.resolve(strict=True)
    output = output.resolve(strict=False)
    try:
        output.relative_to(bundle)
    except ValueError as error:
        raise AuditError("hash index must be inside the audited bundle") from error
    if os.path.lexists(output):
        raise AuditError("refuse to replace an existing hash index")
    index = audit(bundle)
    index["files"] = inventory(bundle, output)
    payload = (json.dumps(index, indent=2, sort_keys=True) + "\n").encode()
    descriptor = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(payload)
        stream.flush()
        os.fsync(stream.fileno())


def verify_index(bundle, index_path):
    bundle = bundle.resolve(strict=True)
    index_path = index_path.resolve(strict=True)
    index = strict_json(index_path)
    if set(index) != {
        "schema_version", "algorithm", "audited_at", "requirements_sha256", "gates", "files"
    }:
        raise AuditError("bundle hash index has unknown or missing fields")
    if index.get("schema_version") != 1 or index.get("algorithm") != "sha256":
        raise AuditError("unsupported bundle hash index")
    if index.get("gates") != list(GATES):
        raise AuditError("bundle hash index does not cover exactly G4, G5, and G6")
    if not isinstance(index.get("audited_at"), str) or not index["audited_at"].endswith("Z"):
        raise AuditError("bundle hash index has no UTC audit timestamp")
    if index.get("requirements_sha256") != sha256(REQUIREMENTS):
        raise AuditError("requirements changed after bundle audit")
    expected = inventory(bundle, index_path)
    if index.get("files") != expected:
        raise AuditError("bundle contents differ from hash index")
    audit(bundle)


def main():
    parser = argparse.ArgumentParser()
    commands = parser.add_subparsers(dest="command", required=True)
    audit_parser = commands.add_parser("audit")
    audit_parser.add_argument("bundle", type=Path)
    audit_parser.add_argument("output", type=Path)
    verify_parser = commands.add_parser("verify")
    verify_parser.add_argument("bundle", type=Path)
    verify_parser.add_argument("index", type=Path)
    arguments = parser.parse_args()
    try:
        if arguments.command == "audit":
            write_index(arguments.bundle, arguments.output)
        else:
            verify_index(arguments.bundle, arguments.index)
    except (OSError, AuditError, KeyError, TypeError) as error:
        print(f"G4-G6 evidence audit: {error}", file=sys.stderr)
        return 1
    print(f"G4-G6 evidence {arguments.command}: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
