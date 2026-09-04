#!/usr/bin/env python3
"""Focused tests for the pre-allocation OCI adapter validation."""

import copy
import json
import os
import pathlib
import subprocess
import tempfile


REPO = pathlib.Path(__file__).resolve().parent.parent
VALIDATOR = REPO / "scripts/validate-runtime-oci.py"
BUILDER = REPO / "scripts/build-runtime-container-initramfs.sh"
BASE = {
    "ociVersion": "1.1.0",
    "process": {
        "terminal": False,
        "user": {"uid": 0, "gid": 0, "additionalGids": [1]},
        "args": ["/bin/true"],
        "env": ["PATH=/bin"],
        "cwd": "/",
    },
    "root": {"path": "rootfs"},
}


def run_case(directory, name, config=None, raw=None, accepted=False):
    source = directory / f"{name}.json"
    output = directory / f"{name}.out.json"
    source.write_text(raw if raw is not None else json.dumps(config), encoding="utf-8")
    result = subprocess.run(
        [str(VALIDATOR), str(source), str(output)],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if (result.returncode == 0) != accepted:
        raise AssertionError(f"{name}: exit={result.returncode}, stderr={result.stderr!r}")
    if accepted and json.loads(output.read_text(encoding="utf-8")) != config:
        raise AssertionError(f"{name}: validator changed accepted input")
    if not accepted and output.exists():
        raise AssertionError(f"{name}: rejected input produced output")


def main():
    with tempfile.TemporaryDirectory(prefix="mk-oci-validation-") as temporary:
        directory = pathlib.Path(temporary)
        run_case(directory, "valid", copy.deepcopy(BASE), accepted=True)
        run_case(
            directory,
            "duplicate",
            raw='{"ociVersion":"1.1.0","ociVersion":"1.1.0","process":{},"root":{}}',
        )
        top_fields = ["mounts", "hooks", "linux", "hostname", "annotations"]
        for field in top_fields:
            config = copy.deepcopy(BASE)
            config[field] = [] if field == "mounts" else {}
            run_case(directory, f"top-{field}", config)
        process_fields = [
            "capabilities", "rlimits", "noNewPrivileges", "consoleSize",
            "apparmorProfile", "oomScoreAdj", "scheduler", "selinuxLabel",
            "ioPriority", "commandLine",
        ]
        for field in process_fields:
            config = copy.deepcopy(BASE)
            config["process"][field] = False if field == "noNewPrivileges" else {}
            run_case(directory, f"process-{field}", config)
        for field, value in (("readonly", False), ("unknown", None)):
            config = copy.deepcopy(BASE)
            config["root"][field] = value
            run_case(directory, f"root-{field}", config)
        for field, value in (("umask", 0o22), ("username", "root"), ("unknown", None)):
            config = copy.deepcopy(BASE)
            config["process"]["user"][field] = value
            run_case(directory, f"user-{field}", config)

        bundle = directory / "bundle"
        (bundle / "rootfs").mkdir(parents=True)
        config = copy.deepcopy(BASE)
        config["annotations"] = {}
        (bundle / "config.json").write_text(json.dumps(config), encoding="utf-8")
        environment = os.environ.copy()
        environment.update({
            "MK_AGENT": str(directory / "missing-agent"),
            "MK_RELAY": str(directory / "missing-relay"),
            "MK_TRANSPORT_MODULE": str(directory / "missing-module"),
        })
        result = subprocess.run(
            [str(BUILDER), str(bundle), str(directory / "initramfs")],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=environment,
            check=False,
        )
        if result.returncode == 0 or "unsupported OCI field(s): annotations" not in result.stderr:
            raise AssertionError(f"builder did not reject before normalization: {result.stderr!r}")
    print("runtime OCI fail-closed validation: PASS (25 cases)")


if __name__ == "__main__":
    main()
