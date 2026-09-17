#!/usr/bin/env python3
"""Focused tests for private read-only bind input materialization."""

import json
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from unittest import mock


SCRIPT = Path(__file__).with_name("materialize-runtime-binds.py")
OPTIONS = ["bind", "ro", "nodev", "nosuid", "noexec"]


def plan(path: Path, source: Path, destination: str = "/opt/input") -> None:
    path.write_text(json.dumps({
        "schema_version": 1,
        "readonly_binds": [{
            "destination": destination,
            "type": "bind",
            "source": str(source),
            "options": OPTIONS,
        }],
    }), encoding="utf-8")


def invoke(plan_path: Path, root: Path, result: Path, environment=None):
    return subprocess.run(
        [str(SCRIPT), str(plan_path), str(root), str(result)],
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=environment, check=False,
    )


def invoke_limited(plan_path: Path, root: Path, result: Path, *limits):
    return subprocess.run(
        [str(SCRIPT), str(plan_path), str(root), str(result), *limits],
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )


def load_materializer():
    spec = importlib.util.spec_from_file_location("runtime_bind_materializer_test", SCRIPT)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    previous = sys.dont_write_bytecode
    sys.dont_write_bytecode = True
    try:
        spec.loader.exec_module(module)
    finally:
        sys.dont_write_bytecode = previous
    return module


def main() -> None:
    with tempfile.TemporaryDirectory(prefix="mk-bind-materialization-") as temporary:
        base = Path(temporary)
        source = base / "source"
        source.mkdir()
        value = source / "value"
        value.write_text("immutable input\n", encoding="utf-8")
        os.chmod(value, 0o640)
        os.link(value, source / "linked")
        os.symlink("value", source / "symbolic")
        root = base / "root"
        root.mkdir()
        plan_path = base / "plan.json"
        result = base / "result.json"
        plan(plan_path, source)
        completed = invoke(plan_path, root, result)
        if completed.returncode != 0:
            raise AssertionError(completed.stderr)
        copied = root / "opt" / "input"
        if (copied / "value").read_text(encoding="utf-8") != "immutable input\n":
            raise AssertionError("materialized bytes differ")
        if (copied / "value").stat().st_ino != (copied / "linked").stat().st_ino:
            raise AssertionError("hardlink identity was not preserved")
        if os.readlink(copied / "symbolic") != "value" or (copied / "value").stat().st_mode & 0o777 != 0o640:
            raise AssertionError("symlink or mode was not preserved")
        record = json.loads(result.read_text(encoding="utf-8"))["readonly_binds"][0]
        if record["destination"] != "/opt/input" or len(record["manifest_sha256"]) != 64:
            raise AssertionError("materialization provenance is incomplete")

        conflict_root = base / "conflict-root"
        (conflict_root / "opt" / "input").mkdir(parents=True)
        (conflict_root / "opt" / "input" / "hidden-by-bind").write_text("old\n", encoding="utf-8")
        conflict = invoke(plan_path, conflict_root, base / "conflict.json")
        if conflict.returncode != 0:
            raise AssertionError(f"real destination replacement failed: {conflict.stderr!r}")
        if (conflict_root / "opt" / "input" / "hidden-by-bind").exists():
            raise AssertionError("bind destination did not hide prior staged content")
        if (conflict_root / "opt" / "input" / "value").read_text(encoding="utf-8") != "immutable input\n":
            raise AssertionError("replacement bind content is missing")

        file_root = base / "file-root"
        (file_root / "opt").mkdir(parents=True)
        (file_root / "opt" / "input").write_text("not-a-directory\n", encoding="utf-8")
        file_result = invoke(plan_path, file_root, base / "file-result.json")
        if file_result.returncode == 0 or "not a real directory" not in file_result.stderr:
            raise AssertionError("non-directory bind destination was accepted")

        limited_root = base / "limited-root"
        limited_root.mkdir()
        limited = invoke_limited(plan_path, limited_root, base / "limited.json", "--max-bytes", "1")
        if limited.returncode == 0 or "aggregate byte or inode limit" not in limited.stderr:
            raise AssertionError("oversized bind input was accepted")

        real_source = base / "real-source"
        real_source.mkdir()
        symlink_source = base / "symlink-source"
        symlink_source.symlink_to(real_source, target_is_directory=True)
        plan(plan_path, symlink_source)
        symlink_root = base / "symlink-root"
        symlink_root.mkdir()
        symlink_result = invoke(plan_path, symlink_root, base / "symlink.json")
        if symlink_result.returncode == 0 or "real directories" not in symlink_result.stderr:
            raise AssertionError("symlinked bind source was accepted")

        mutation_source = base / "mutation-source"
        mutation_source.mkdir()
        mutation_value = mutation_source / "value"
        mutation_value.write_text("before\n", encoding="utf-8")
        mutation_root = base / "mutation-root"
        mutation_root.mkdir()
        plan(plan_path, mutation_source)
        materializer = load_materializer()
        real_run = subprocess.run

        def mutate_after_copy(command, **kwargs):
            completed = real_run(command, **kwargs)
            mutation_value.write_text("after\n", encoding="utf-8")
            return completed

        arguments = [str(SCRIPT), str(plan_path), str(mutation_root), str(base / "mutation.json")]
        with mock.patch.object(materializer.subprocess, "run", side_effect=mutate_after_copy), mock.patch.object(sys, "argv", arguments):
            try:
                materializer.main()
            except materializer.MaterializationError as error:
                if "mutated during materialization" not in str(error):
                    raise
            else:
                raise AssertionError("source mutation was accepted")

    print("runtime read-only bind materialization: PASS (copy, metadata, destination semantics, symlinks, mutation)")


if __name__ == "__main__":
    main()
