#!/usr/bin/env python3
import os
from pathlib import Path
import subprocess
import tempfile
import tomllib


REPO = Path(__file__).resolve().parent.parent
FRAGMENT = REPO / "deploy/containerd/20-multikernel-runtime.toml"
value = tomllib.loads(FRAGMENT.read_text())
assert set(value) == {"plugins"}
plugins = value["plugins"]
assert set(plugins) == {"io.containerd.cri.v1.runtime"}
containerd = plugins["io.containerd.cri.v1.runtime"]["containerd"]
assert set(containerd) == {"runtimes"}
runtimes = containerd["runtimes"]
assert set(runtimes) == {"multikernel"}
runtime = runtimes["multikernel"]
assert runtime["runtime_type"] == "io.containerd.multikernel.v2"

# When containerd is locally available, parse the real imported configuration
# and prove the existing runc default plus handler coexist. This test is
# optional on documentation-only machines, but mandatory on the GCE host.
if Path("/usr/bin/containerd").exists():
    with tempfile.TemporaryDirectory() as raw:
        directory = Path(raw)
        imported = directory / "20-multikernel-runtime.toml"
        imported.symlink_to(FRAGMENT)
        main = directory / "config.toml"
        main.write_text(
            "version = 4\nimports = [" + repr(os.fspath(imported)) + "]\n"
            "[plugins.'io.containerd.cri.v1.runtime'.containerd]\n"
            "  default_runtime_name = 'runc'\n"
            "[plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.runc]\n"
            "  runtime_type = 'io.containerd.runc.v2'\n"
        )
        completed = subprocess.run(
            ["/usr/bin/containerd", "--config", os.fspath(main), "config", "dump"],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
        )
        assert completed.returncode == 0, completed.stderr
        dumped = tomllib.loads(completed.stdout)
        configured = dumped["plugins"]["io.containerd.cri.v1.runtime"]["containerd"]
        assert configured["default_runtime_name"] == "runc"
        assert configured["runtimes"]["runc"]["runtime_type"] == "io.containerd.runc.v2"
        assert configured["runtimes"]["multikernel"]["runtime_type"] == "io.containerd.multikernel.v2"

print("runtime containerd config tests: PASS")
