#!/usr/bin/env python3
import importlib.util
import json
from pathlib import Path
import sys
import tempfile


SCRIPT = Path(__file__).with_name("runtime-release-manifest.py")
old_dont_write_bytecode = sys.dont_write_bytecode
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("runtime_release_manifest", SCRIPT)
module = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(module)
sys.dont_write_bytecode = old_dont_write_bytecode


def component(directory: Path, name: str, version: str, revision: str) -> None:
    path = directory / name
    path.write_text(
        "#!/bin/sh\nprintf '%s\\n' "
        + repr(f"{name} version={version} revision={revision}")
        + "\n"
    )
    path.chmod(0o755)


revision = "0123456789abcdef0123456789abcdef01234567"
with tempfile.TemporaryDirectory() as raw:
    directory = Path(raw)
    for name in module.COMPONENTS:
        component(directory, name, "1.2.3", revision)
    first = module.create(directory, "1.2.3", revision)
    second = module.create(directory, "1.2.3", revision)
    assert first == second
    assert [item["name"] for item in first["components"]] == list(module.COMPONENTS)
    assert all(len(item["sha256"]) == 64 for item in first["components"])
    (directory / "mk-agent").unlink()
    (directory / "mk-agent").symlink_to("mk-agentctl")
    try:
        module.create(directory, "1.2.3", revision)
        raise AssertionError("symlink component was accepted")
    except ValueError as error:
        assert "non-symlink" in str(error)
    (directory / "mk-agent").unlink()
    component(directory, "mk-agent", "9.9.9", revision)
    try:
        module.create(directory, "1.2.3", revision)
        raise AssertionError("mismatched version was accepted")
    except ValueError as error:
        assert "does not match" in str(error)
    try:
        module.create(directory, "1.2.3", "bad")
        raise AssertionError("malformed revision was accepted")
    except ValueError as error:
        assert "40-hex" in str(error)

print("runtime release manifest tests: PASS")
