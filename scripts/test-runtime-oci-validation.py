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
    "linux": {"namespaces": [
        {"type": "mount"}, {"type": "pid"}, {"type": "network", "path": "/run/netns/test"},
    ]},
    "annotations": {"io.example.test": "inert"},
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
    if accepted:
        projected = json.loads(output.read_text(encoding="utf-8"))
        expected = {key: config[key] for key in ("ociVersion", "process", "root")}
        expected["ociVersion"] = "1.1.0"
        if config.get("hostname"):
            expected["hostname"] = config["hostname"]
        policy = {name: config["linux"][name] for name in ("maskedPaths", "readonlyPaths") if name in config["linux"]}
        if policy:
            expected["linux"] = policy
        if projected != expected:
            raise AssertionError(f"{name}: unexpected guest projection {projected!r}")
    if not accepted and output.exists():
        raise AssertionError(f"{name}: rejected input produced output")


def main():
    with tempfile.TemporaryDirectory(prefix="mk-oci-validation-") as temporary:
        directory = pathlib.Path(temporary)
        run_case(directory, "valid", copy.deepcopy(BASE), accepted=True)
        supported = copy.deepcopy(BASE)
        supported["ociVersion"] = "1.0.2-dev"
        supported["process"].update({
            "noNewPrivileges": True,
            "rlimits": [{"type": "RLIMIT_NOFILE", "soft": 64, "hard": 64}],
            "capabilities": {"bounding": ["CAP_CHOWN"], "permitted": ["CAP_CHOWN"], "effective": ["CAP_CHOWN"]},
        })
        run_case(directory, "supported-process-controls", supported, accepted=True)
        run_case(
            directory,
            "duplicate",
            raw='{"ociVersion":"1.1.0","ociVersion":"1.1.0","process":{},"root":{}}',
        )
        run_case(directory, "truncated-json", raw='{"ociVersion":"1.1.0",')
        top_fields = ["hooks"]
        for field in top_fields:
            config = copy.deepcopy(BASE)
            config[field] = [] if field == "mounts" else {}
            run_case(directory, f"top-{field}", config)
        standard = copy.deepcopy(BASE)
        standard["ociVersion"] = "1.3.0"
        standard["hostname"] = "sandbox-one"
        standard["root"]["readonly"] = False
        standard["mounts"] = [
            {"destination": destination, "type": mount_type, "source": source, "options": sorted(options)}
            for destination, (mount_type, source, options) in {
                "/proc": ("proc", "proc", {"nosuid", "noexec", "nodev"}),
                "/dev": ("tmpfs", "tmpfs", {"nosuid", "strictatime", "mode=755", "size=65536k"}),
                "/dev/pts": ("devpts", "devpts", {"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"}),
                "/dev/shm": ("tmpfs", "shm", {"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"}),
                "/dev/mqueue": ("mqueue", "mqueue", {"nosuid", "noexec", "nodev"}),
                "/sys": ("sysfs", "sysfs", {"nosuid", "noexec", "nodev", "ro"}),
                "/run": ("tmpfs", "tmpfs", {"nosuid", "strictatime", "mode=755", "size=65536k"}),
            }.items()
        ]
        standard["linux"].update({
            "resources": {"devices": [{"allow": False, "access": "rwm"}]},
            "cgroupsPath": "/default/task",
            "maskedPaths": ["/proc/kcore", "/sys/firmware"],
            "readonlyPaths": ["/proc/sys"],
        })
        run_case(directory, "standard-containerd", standard, accepted=True)
        bad_mount = copy.deepcopy(standard)
        bad_mount["mounts"][0]["options"].append("rw")
        run_case(directory, "modified-default-mount", bad_mount)
        bad_resource = copy.deepcopy(standard)
        bad_resource["linux"]["resources"]["memory"] = {"limit": 1}
        run_case(directory, "unsupported-resource", bad_resource)
        for name, namespaces in (
            ("missing-network", [{"type": "mount"}]),
            ("duplicate-network", [{"type": "network"}, {"type": "network"}]),
            ("joined-pid", [{"type": "network"}, {"type": "pid", "path": "/proc/1/ns/pid"}]),
            ("user-namespace", [{"type": "network"}, {"type": "user"}]),
            ("relative-network", [{"type": "network", "path": "relative"}]),
        ):
            config = copy.deepcopy(BASE)
            config["linux"]["namespaces"] = namespaces
            run_case(directory, name, config)
        process_fields = [
            "consoleSize",
            "apparmorProfile", "oomScoreAdj", "scheduler", "selinuxLabel",
            "ioPriority", "commandLine",
        ]
        for field in process_fields:
            config = copy.deepcopy(BASE)
            config["process"][field] = False if field == "noNewPrivileges" else {}
            run_case(directory, f"process-{field}", config)
        for name, mutate in (
            ("bad-nnp", lambda process: process.update(noNewPrivileges="yes")),
            ("bad-rlimit", lambda process: process.update(rlimits=[{"type": "RLIMIT_NOFILE", "soft": 2, "hard": 1}])),
            ("unknown-capability", lambda process: process.update(capabilities={"effective": ["CAP_UNKNOWN"]})),
            ("duplicate-capability", lambda process: process.update(capabilities={"effective": ["CAP_CHOWN", "CAP_CHOWN"]})),
        ):
            config = copy.deepcopy(BASE)
            mutate(config["process"])
            run_case(directory, name, config)
        for field, value in (("readonly", "false"), ("unknown", None)):
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
        config["hooks"] = {"prestart": []}
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
        if result.returncode == 0 or "unsupported OCI field(s): hooks" not in result.stderr:
            raise AssertionError(f"builder did not reject before normalization: {result.stderr!r}")
    print("runtime OCI fail-closed validation: PASS (29 cases plus namespace projection)")


if __name__ == "__main__":
    main()
