#!/usr/bin/env python3
"""Exercise implicit-module/site/bytecode routes and isolated source execution."""

import argparse
import importlib.util
import json
import os
from pathlib import Path
import py_compile
import shutil
import struct
import subprocess
import sys
import tarfile
import tempfile

REPO = Path(__file__).resolve().parent.parent


def poison_cache(source, canary):
    poison = source.parent / "synthetic-cache-input"
    poison.write_text(f"from pathlib import Path\nPath({str(canary)!r}).write_text('synthetic-bytecode')\nraise SystemExit(0)\n")
    cache = Path(importlib.util.cache_from_source(str(source)))
    cache.parent.mkdir(exist_ok=True)
    py_compile.compile(str(poison), cfile=str(cache), dfile=str(source), doraise=True,
                       invalidation_mode=py_compile.PycInvalidationMode.TIMESTAMP)
    content = bytearray(cache.read_bytes())
    info = source.stat()
    content[8:16] = struct.pack("<II", int(info.st_mtime) & 0xffffffff, info.st_size & 0xffffffff)
    cache.write_bytes(content)
    poison.unlink()


CASES = ("sibling-module", "user-site", "publication-bytecode", "nested-rootfs-bytecode")


def qualify(legacy=None, cases=CASES):
    for case in cases:
        with tempfile.TemporaryDirectory(prefix="mk-builder-isolation-") as temporary:
            root = Path(temporary)
            generation = root / "generation"
            generation.mkdir(mode=0o700)
            for source in (REPO / "scripts").glob("*.py"):
                shutil.copyfile(source, generation / source.name)
                (generation / source.name).chmod(0o755)
            builder = generation / "build-runtime-container-initramfs.sh"
            shutil.copyfile(REPO / "scripts" / builder.name, builder)
            builder.chmod(0o755)
            if legacy:
                with tarfile.open(legacy) as archive:
                    for member in archive.getmembers():
                        if member.isfile() and member.name.startswith("scripts/"):
                            destination = generation / Path(member.name).name
                            if destination.exists():
                                destination.write_bytes(archive.extractfile(member).read())
            canary = root / "implicit-code-executed"
            environment = {"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C", "LC_ALL": "C"}
            result_path = root / "result.json"
            if case in ("sibling-module", "user-site"):
                bundle = root / "bundle"
                bundle.mkdir()
                config = {"ociVersion": "1.1.0", "process": {"terminal": False,
                    "user": {"uid": 0, "gid": 0}, "args": ["/bin/true"], "env": [], "cwd": "/"},
                    "root": {"path": "rootfs"}, "linux": {"namespaces": [
                        {"type": "mount"}, {"type": "pid"}, {"type": "network"}]}}
                (bundle / "config.json").write_text(json.dumps(config))
                (bundle / "config.json").chmod(0o600)
                environment["MK_VALIDATE_ONLY"] = "1"
                command = [str(builder), str(bundle), str(root / "initramfs.cpio.gz")]
                if case == "sibling-module":
                    (generation / "json.py").write_text(f"from pathlib import Path\nPath({str(canary)!r}).write_text('synthetic-sibling')\nraise SystemExit(0)\n")
                else:
                    userbase = root / "userbase"
                    site = userbase / f"lib/python{sys.version_info.major}.{sys.version_info.minor}/site-packages"
                    site.mkdir(parents=True)
                    (site / "usercustomize.py").write_text(f"from pathlib import Path\nPath({str(canary)!r}).write_text('synthetic-user-site')\n")
                    environment["PYTHONUSERBASE"] = str(userbase)
            else:
                payload = root / "payload"
                payload.mkdir()
                (payload / "data").write_bytes(b"preserved-payload")
                helper = "build-runtime-rootfs.py"
                arguments = [str(payload), str(root / "unused"), str(result_path), "--manifest-only"]
                if case == "publication-bytecode":
                    poison_cache(generation / "runtime_safe_publish.py", canary)
                else:
                    poison_cache(generation / "build-runtime-rootfs.py", canary)
                    helper = "materialize-runtime-binds.py"
                    plan = root / "plan.json"
                    plan.write_text(json.dumps({"schema_version": 1, "readonly_binds": []}))
                    arguments = [str(plan), str(payload), str(result_path)]
                command = (["/usr/bin/python3", str(generation / helper)] if legacy else
                           ["/usr/bin/python3", "-I", "-S", str(generation / "runtime_support_exec.py"), helper]) + arguments
            result = subprocess.run(command, env=environment, capture_output=True, text=True)
            assert result.returncode == 0 and not canary.exists(), f"{case}: exit={result.returncode} canary={canary.exists()} stderr={result.stderr!r}"
            if case in ("sibling-module", "user-site"):
                assert json.loads(result.stdout)["validated"]
            else:
                json.loads(result_path.read_text())
                assert (payload / "data").read_bytes() == b"preserved-payload"
            print(case + ": PASS actual code route excluded, legitimate helper preserved", flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--legacy", type=Path)
    parser.add_argument("--case", choices=CASES)
    args = parser.parse_args()
    qualify(args.legacy, (args.case,) if args.case else CASES)
