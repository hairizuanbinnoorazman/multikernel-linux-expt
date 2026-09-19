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
        if file_result.returncode == 0 or "type differs" not in file_result.stderr:
            raise AssertionError("mismatched bind destination type was accepted")

        regular_source = base / "regular-source"
        regular_source.write_text("regular immutable input\n", encoding="utf-8")
        regular_source.chmod(0o640)
        regular_root = base / "regular-root"
        (regular_root / "etc").mkdir(parents=True)
        regular_target = regular_root / "etc" / "input.conf"
        regular_target.write_text("hidden old file\n", encoding="utf-8")
        plan(plan_path, regular_source, "/etc/input.conf")
        regular_result = invoke(plan_path, regular_root, base / "regular.json")
        if regular_result.returncode != 0:
            raise AssertionError(f"regular-file bind failed: {regular_result.stderr!r}")
        if regular_target.read_text(encoding="utf-8") != "regular immutable input\n" or regular_target.stat().st_mode & 0o777 != 0o640:
            raise AssertionError("regular-file bind bytes or mode differ")

        linked_source = base / "linked-source"
        os.link(regular_source, linked_source)
        linked_root = base / "linked-root"
        linked_root.mkdir()
        plan(plan_path, linked_source, "/input.conf")
        linked_result = invoke(plan_path, linked_root, base / "linked.json")
        if linked_result.returncode == 0 or "single-link regular file" not in linked_result.stderr:
            raise AssertionError("hard-linked regular-file source was accepted")
        linked_source.unlink()

        limited_root = base / "limited-root"
        limited_root.mkdir()
        plan(plan_path, regular_source, "/input.conf")
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
        if symlink_result.returncode == 0 or "must not traverse" not in symlink_result.stderr:
            raise AssertionError("symlinked bind source was accepted")

        race_source = base / "race-source"
        race_source.mkdir()
        (race_source / "value").write_text("original\n", encoding="utf-8")
        race_root = base / "race-root"
        race_root.mkdir()
        race_result = base / "race.json"
        race_moved = base / "race-source-held"
        plan(plan_path, race_source)
        materializer = load_materializer()
        real_run = subprocess.run

        def replace_before_copy(command, **kwargs):
            race_source.rename(race_moved)
            race_source.mkdir()
            (race_source / "value").write_text("replacement\n", encoding="utf-8")
            return real_run(command, **kwargs)

        arguments = [str(SCRIPT), str(plan_path), str(race_root), str(race_result)]
        with mock.patch.object(materializer.subprocess, "run", side_effect=replace_before_copy), mock.patch.object(sys, "argv", arguments):
            materializer.main()
        if (race_root / "opt" / "input" / "value").read_text(encoding="utf-8") != "original\n":
            raise AssertionError("bind copy escaped the held source descriptor")
        if (race_source / "value").read_text(encoding="utf-8") != "replacement\n":
            raise AssertionError("public bind-source replacement was modified")

        target_source = base / "target-race-source"
        target_source.mkdir()
        (target_source / "value").write_text("held target\n", encoding="utf-8")
        target_root = base / "target-race-root"
        target_root.mkdir()
        target_moved = base / "target-race-root-held"
        plan(plan_path, target_source)
        materializer = load_materializer()
        real_destination_path = materializer.destination_path

        def replace_target_root(held_root, destination, source_is_directory):
            target_root.rename(target_moved)
            target_root.mkdir()
            (target_root / "replacement-marker").write_text("preserve\n", encoding="utf-8")
            return real_destination_path(held_root, destination, source_is_directory)

        arguments = [str(SCRIPT), str(plan_path), str(target_root), str(base / "target-race.json")]
        with mock.patch.object(materializer, "destination_path", side_effect=replace_target_root), mock.patch.object(sys, "argv", arguments):
            materializer.main()
        if (target_moved / "opt" / "input" / "value").read_text(encoding="utf-8") != "held target\n":
            raise AssertionError("bind copy escaped the held target descriptor")
        if (target_root / "replacement-marker").read_text(encoding="utf-8") != "preserve\n":
            raise AssertionError("public target-root replacement was modified")

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

    print("runtime read-only bind materialization: PASS (copy, metadata, destination semantics, symlinks, mutation, held source/target races)")


if __name__ == "__main__":
    main()
