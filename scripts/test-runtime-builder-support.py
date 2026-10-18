#!/usr/bin/env python3
"""Qualify builder support preflight before synthetic helper/import execution."""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

REPO = Path(__file__).resolve().parent.parent
SUPPORT = (
    "validate-runtime-oci.py", "validate-runtime-bootstrap.py",
    "validate-runtime-root.py", "validate-runtime-image.py",
    "build-runtime-rootfs.py", "build-runtime-storage.py",
    "materialize-runtime-binds.py", "runtime-storage-identity.py",
    "verify-runtime-rootfs.py", "runtime_safe_publish.py",
    "runtime_support_exec.py",
)


CASES = ("protected", "writable-helper", "writable-import", "symlink", "hardlink",
         "empty", "oversize", "writable-generation", "writable-ancestor", "missing", "directory")


def qualify(builder, cases=CASES):
    for case in cases:
        with tempfile.TemporaryDirectory(prefix="mk-builder-support-") as temporary:
            root = Path(temporary)
            generation = root / "generation"
            generation.mkdir(mode=0o700)
            for name in SUPPORT:
                destination = generation / name
                shutil.copyfile(REPO / "scripts" / name, destination)
                destination.chmod(0o755 if name != "runtime_safe_publish.py" else 0o644)
            selected = generation / "build-runtime-container-initramfs.sh"
            shutil.copyfile(builder, selected)
            selected.chmod(0o755)
            bundle = root / "bundle"
            bundle.mkdir()
            config = {"ociVersion": "1.1.0", "process": {
                "terminal": False, "user": {"uid": 0, "gid": 0},
                "args": ["/bin/true"], "env": [], "cwd": "/"},
                "root": {"path": "rootfs"}, "linux": {"namespaces": [
                    {"type": "mount"}, {"type": "pid"}, {"type": "network"}]}}
            (bundle / "config.json").write_text(json.dumps(config))
            (bundle / "config.json").chmod(0o600)
            canary = root / "untrusted-code-executed"
            imported = generation / "runtime_safe_publish.py"
            validator = generation / "validate-runtime-oci.py"
            if case == "writable-helper":
                validator.write_text(f"#!/bin/sh\ntouch '{canary}'\n")
                validator.chmod(0o777)
            elif case == "writable-import":
                validator.write_text("#!/usr/bin/env python3\nimport runtime_safe_publish\n")
                imported.write_text(f"from pathlib import Path\nPath({str(canary)!r}).write_text('synthetic-import')\n")
                imported.chmod(0o666)
            elif case == "symlink":
                imported.rename(root / "module")
                imported.symlink_to(root / "module")
            elif case == "hardlink":
                os.link(imported, root / "module")
            elif case == "empty":
                imported.write_bytes(b"")
            elif case == "oversize":
                with imported.open("r+b") as stream:
                    stream.truncate(4194305)
            elif case == "writable-generation":
                generation.chmod(0o777)
            elif case == "writable-ancestor":
                root.chmod(0o777)
            elif case == "missing":
                imported.unlink()
            elif case == "directory":
                imported.unlink()
                imported.mkdir()
            elif case == "foreign-owner":
                # Only a root caller explicitly requesting this fixture can
                # change ownership, and only on its private temporary module.
                os.chown(imported, 65534, 65534)
            result = subprocess.run([str(selected), str(bundle), str(root / "initramfs.cpio.gz")],
                env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C",
                     "MK_VALIDATE_ONLY": "1"}, capture_output=True, text=True)
            if case == "protected":
                assert result.returncode == 0 and json.loads(result.stdout)["validated"], result.stderr
                print(case + ": PASS real OCI validator", flush=True)
            else:
                assert result.returncode != 0 and "unsafe builder support generation" in result.stderr and not canary.exists(), (
                    f"{case}: exit={result.returncode} canary={canary.exists()} stderr={result.stderr!r}")
                print(case + ": PASS refusal before helper execution", flush=True)
            assert not (root / "initramfs.cpio.gz").exists()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--builder", type=Path, default=REPO / "scripts/build-runtime-container-initramfs.sh")
    parser.add_argument("--case", choices=CASES)
    parser.add_argument("--root-owner-fixture", action="store_true")
    args = parser.parse_args()
    if args.root_owner_fixture and os.geteuid() != 0:
        parser.error("--root-owner-fixture requires root")
    cases = (args.case,) if args.case else CASES
    if args.root_owner_fixture:
        cases += ("foreign-owner",)
    qualify(args.builder, cases)
