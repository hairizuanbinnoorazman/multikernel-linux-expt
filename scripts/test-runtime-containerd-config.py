#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import tempfile
import tomllib
import re


REPO = Path(__file__).resolve().parent.parent
FRAGMENT = REPO / "deploy/containerd/20-multikernel-runtime.toml"
value = tomllib.loads(FRAGMENT.read_text())
assert set(value) == {"plugins"}
plugins = value["plugins"]
assert set(plugins) == {"io.containerd.cri.v1.runtime"}
cri = plugins["io.containerd.cri.v1.runtime"]
assert set(cri) == {"containerd", "enable_criu", "enable_experimental_restore_via_create"}
assert cri["enable_criu"] is False
assert cri["enable_experimental_restore_via_create"] is False
containerd = cri["containerd"]
assert set(containerd) == {"runtimes"}
runtimes = containerd["runtimes"]
assert set(runtimes) == {"multikernel"}
runtime = runtimes["multikernel"]
assert runtime["runtime_type"] == "io.containerd.multikernel.v2"

# When containerd is locally available, parse the real imported configuration
# and prove the existing runc default plus handler coexist. This test is
# optional on documentation-only machines, but mandatory on the GCE host.
binary = Path(os.environ.get("MK_CONTAINERD", "/usr/bin/containerd"))
if "MK_CONTAINERD" in os.environ:
    assert binary.is_file() and os.access(binary, os.X_OK), "selected containerd is unavailable or not executable"
if binary.exists():
    version = subprocess.check_output([os.fspath(binary), "--version"], text=True)
    matched = re.search(r"\s[v]?(\d+)\.(\d+)\.(\d+)(?:\s|$)", version)
    assert matched, "cannot identify containerd version"
    release = tuple(map(int, matched.groups()))
    assert (release[:2] == (2, 2) and release >= (2, 2, 10)) or (
        release[:2] == (2, 3) and release >= (2, 3, 7)
    ), "containerd release requires separate security qualification"
    with tempfile.TemporaryDirectory() as raw:
        directory = Path(raw)
        imported = directory / "20-multikernel-runtime.toml"
        imported.symlink_to(FRAGMENT)
        main = directory / "config.toml"
        main.write_text(
            "version = " + ("3" if release[:2] == (2, 2) else "4") +
            "\nimports = [" + repr(os.fspath(imported)) + "]\n"
            "[plugins.'io.containerd.cri.v1.runtime'.containerd]\n"
            "  default_runtime_name = 'runc'\n"
            "[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.runc]\n"
            "  runtime_type = 'io.containerd.runc.v2'\n"
        )
        completed = subprocess.run(
            [os.fspath(binary), "--config", os.fspath(main), "config", "dump"],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        assert completed.returncode == 0, completed.stderr
        dumped = tomllib.loads(completed.stdout)
        effective = dumped["plugins"]["io.containerd.cri.v1.runtime"]
        assert effective["enable_criu"] is False
        assert effective["enable_experimental_restore_via_create"] is False
        configured = dumped["plugins"]["io.containerd.cri.v1.runtime"]["containerd"]
        assert configured["default_runtime_name"] == "runc"
        assert configured["runtimes"]["runc"]["runtime_type"] == "io.containerd.runc.v2"
        assert configured["runtimes"]["multikernel"]["runtime_type"] == "io.containerd.multikernel.v2"

print("runtime containerd config tests: PASS")
