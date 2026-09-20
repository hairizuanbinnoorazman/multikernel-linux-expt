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
CASE_COUNT = 0
CONTAINERD_DEFAULT_DEVICES = [
    {"allow": False, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 3, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 8, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 7, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 0, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 5, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 9, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 1, "access": "rwm"},
    {"allow": True, "type": "c", "major": 136, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 2, "access": "rwm"},
]


def run_case(directory, name, config=None, raw=None, accepted=False):
    global CASE_COUNT
    CASE_COUNT += 1
    source = directory / f"{name}.json"
    output = directory / f"{name}.out.json"
    source.write_text(raw if raw is not None else json.dumps(config), encoding="utf-8")
    source.chmod(0o644)
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
        readonly_binds = [item for item in config.get("mounts", []) if item["destination"] not in {
            "/proc", "/dev", "/dev/pts", "/dev/shm", "/dev/mqueue", "/sys", "/run",
        }]
        if readonly_binds:
            expected["mounts"] = [{
                "destination": item["destination"],
                "type": "bind",
                "source": item["destination"],
                "options": ["bind", "nodev", "noexec", "nosuid", "ro"],
            } for item in readonly_binds]
        if projected != expected:
            raise AssertionError(f"{name}: unexpected guest projection {projected!r}")
    if not accepted and output.exists():
        raise AssertionError(f"{name}: rejected input produced output")


def main():
    with tempfile.TemporaryDirectory(prefix="mk-oci-validation-") as temporary:
        directory = pathlib.Path(temporary)
        builder_text = BUILDER.read_text(encoding="utf-8")
        if '$source_manifest.after' in builder_text or 'mv "$source_manifest' in builder_text:
            raise AssertionError("outer builder reintroduced replacing source-manifest publication")
        if '"$source_manifest" --manifest-only' not in builder_text or 'cmp -s "$source_before_manifest" "$source_manifest"' not in builder_text:
            raise AssertionError("outer builder does not compare a no-replace final source manifest")
        missing_bundle = directory / "missing-config-bundle"
        missing_bundle.mkdir()
        output = directory / "initramfs.cpio.gz"
        storage = directory / "root.ext4"
        protected = [
            output,
            directory / "initramfs.manifest.json",
            directory / "initramfs.source-manifest.json",
            directory / "initramfs.source-manifest.json.before",
            directory / "initramfs.source-manifest.json.after",
            directory / "initramfs.readonly-binds.manifest.json",
            storage,
            directory / "storage.json",
        ]
        for index, path in enumerate(protected):
            path.write_bytes(f"replacement-{index}\n".encode())
        environment = os.environ.copy()
        environment["MK_STORAGE_OUTPUT"] = str(storage)
        failed_build = subprocess.run(
            [str(BUILDER), str(missing_bundle), str(output)],
            env=environment,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if failed_build.returncode == 0:
            raise AssertionError("builder unexpectedly accepted a bundle without config.json")
        for index, path in enumerate(protected):
            expected = f"replacement-{index}\n".encode()
            if path.read_bytes() != expected:
                raise AssertionError(f"outer failure cleanup modified replacement {path}")

        run_case(directory, "valid", copy.deepcopy(BASE), accepted=True)
        inherited_bundle = directory / "inherited-bundle"
        inherited_bundle.mkdir(mode=0o700)
        inherited_config = inherited_bundle / "config.json"
        inherited_config.write_text(json.dumps(BASE), encoding="utf-8")
        inherited_config.chmod(0o600)
        inherited_output = directory / "inherited-output.json"
        inherited_fd = os.open(inherited_bundle, os.O_RDONLY | os.O_DIRECTORY)
        try:
            inherited = subprocess.run(
                [str(VALIDATOR), f"/proc/self/fd/{inherited_fd}/config.json", str(inherited_output)],
                pass_fds=(inherited_fd,), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                text=True, check=False,
            )
        finally:
            os.close(inherited_fd)
        if inherited.returncode != 0 or json.loads(inherited_output.read_text(encoding="utf-8"))["process"] != BASE["process"]:
            raise AssertionError(f"inherited bundle config failed: {inherited.stderr!r}")
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
        current_containerd = copy.deepcopy(standard)
        current_containerd["linux"]["resources"]["devices"] = copy.deepcopy(CONTAINERD_DEFAULT_DEVICES)
        run_case(directory, "current-containerd-default-devices", current_containerd, accepted=True)
        ctr_default = copy.deepcopy(current_containerd)
        ctr_default["linux"]["resources"]["cpu"] = {"shares": 1024}
        run_case(directory, "ctr-default-cpu-shares", ctr_default, accepted=True)
        for name, mutate in (
            ("containerd-devices-missing", lambda devices: devices.pop()),
            ("containerd-devices-reordered", lambda devices: devices.reverse()),
            ("containerd-devices-custom", lambda devices: devices.append(
                {"allow": True, "type": "b", "major": 8, "access": "rwm"})),
        ):
            config = copy.deepcopy(current_containerd)
            mutate(config["linux"]["resources"]["devices"])
            run_case(directory, name, config)
        for name, cpu in (
            ("nondefault-cpu-shares", {"shares": 512}),
            ("cpu-quota", {"shares": 1024, "quota": 10000}),
        ):
            config = copy.deepcopy(current_containerd)
            config["linux"]["resources"]["cpu"] = cpu
            run_case(directory, name, config)
        readonly_bind = copy.deepcopy(BASE)
        readonly_bind["mounts"] = [{
            "destination": "/opt/input", "type": "bind", "source": "/srv/input",
            "options": ["rprivate", "ro", "rbind"],
        }]
        run_case(directory, "docker-readonly-bind", readonly_bind, accepted=True)
        hardened_bind = copy.deepcopy(readonly_bind)
        hardened_bind["mounts"][0]["options"] = ["noexec", "ro", "bind", "nosuid", "nodev"]
        run_case(directory, "hardened-readonly-bind", hardened_bind, accepted=True)
        bind_source = directory / "readonly-bind-plan-source.json"
        bind_guest = directory / "readonly-bind-guest.json"
        bind_plan = directory / "readonly-bind-plan.json"
        bind_source.write_text(json.dumps(readonly_bind), encoding="utf-8")
        bind_source.chmod(0o644)
        projected = subprocess.run(
            [str(VALIDATOR), str(bind_source), str(bind_guest), str(bind_plan)],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False,
        )
        if projected.returncode != 0:
            raise AssertionError(f"bind projections failed: {projected.stderr!r}")
        guest_mount = json.loads(bind_guest.read_text(encoding="utf-8"))["mounts"][0]
        host_mount = json.loads(bind_plan.read_text(encoding="utf-8"))["readonly_binds"][0]
        if guest_mount["source"] != "/opt/input" or host_mount["source"] != "/srv/input":
            raise AssertionError("host bind source crossed into the guest projection")
        if host_mount["options"] != ["bind", "ro", "nodev", "nosuid", "noexec"]:
            raise AssertionError("host bind plan was not canonicalized")
        for name, mutate in (
            ("writable-bind", lambda mount: mount["options"].remove("ro")),
            ("shared-bind-propagation", lambda mount: mount["options"].append("rshared")),
            ("conflicting-private-bind", lambda mount: mount["options"].append("private")),
            ("relative-bind-source", lambda mount: mount.update(source="srv/input")),
            ("host-root-bind-source", lambda mount: mount.update(source="/")),
            ("noncanonical-bind-destination", lambda mount: mount.update(destination="/opt/../input")),
            ("protected-bind-destination", lambda mount: mount.update(destination="/proc/input")),
            ("unsupported-bind-type", lambda mount: mount.update(type="tmpfs")),
        ):
            config = copy.deepcopy(readonly_bind)
            mutate(config["mounts"][0])
            run_case(directory, name, config)
        overlapping_binds = copy.deepcopy(readonly_bind)
        overlapping_binds["mounts"].append({
            "destination": "/opt/input/nested", "type": "bind", "source": "/srv/nested",
            "options": ["bind", "ro", "nodev", "nosuid", "noexec"],
        })
        run_case(directory, "overlapping-binds", overlapping_binds)
        bad_mount = copy.deepcopy(standard)
        bad_mount["mounts"][0]["options"].append("rw")
        run_case(directory, "modified-default-mount", bad_mount)
        bad_resource = copy.deepcopy(standard)
        bad_resource["linux"]["resources"]["memory"] = {"limit": 1}
        run_case(directory, "unsupported-resource", bad_resource)
        for name, linux_update in (
            ("unsupported-seccomp", {"seccomp": {"defaultAction": "SCMP_ACT_ERRNO"}}),
            ("noncanonical-cgroup", {"cgroupsPath": "/tasks/../escape"}),
            ("unsupported-masked-path", {"maskedPaths": ["/etc/shadow"]}),
        ):
            config = copy.deepcopy(BASE)
            config["linux"].update(linux_update)
            run_case(directory, name, config)
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
        for name, mutate in (
            ("empty-argv0", lambda process: process.update(args=[""])),
            ("nul-argument", lambda process: process.update(args=["/bin/true", "bad\0arg"])),
            ("too-many-arguments", lambda process: process.update(args=["/bin/true"] * 257)),
            ("oversized-process-text", lambda process: process.update(args=["/bin/true", "x" * (128 << 10)])),
            ("invalid-environment", lambda process: process.update(env=["NOVALUE"])),
            ("duplicate-environment", lambda process: process.update(env=["A=1", "A=2"])),
            ("nul-environment", lambda process: process.update(env=["A=bad\0value"])),
            ("noncanonical-cwd", lambda process: process.update(cwd="/work/../escape")),
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
        for name, gids in (("duplicate-gids", [1, 1]), ("too-many-gids", list(range(257))), ("malformed-gid", [{}])):
            config = copy.deepcopy(BASE)
            config["process"]["user"]["additionalGids"] = gids
            run_case(directory, name, config)
        invalid_hostname = copy.deepcopy(BASE)
        invalid_hostname["hostname"] = "-invalid"
        run_case(directory, "invalid-hostname", invalid_hostname)

        secure_source = directory / "secure-source.json"
        secure_output = directory / "secure-output.json"
        secure_source.write_text(json.dumps(BASE), encoding="utf-8")
        secure_source.chmod(0o644)
        hardlink = directory / "hardlink-source.json"
        os.link(secure_source, hardlink)
        result = subprocess.run([str(VALIDATOR), str(hardlink), str(secure_output)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode == 0 or secure_output.exists():
            raise AssertionError("hard-linked OCI config was accepted")
        hardlink.unlink()
        secure_source.unlink()

        writable = directory / "writable-source.json"
        writable.write_text(json.dumps(BASE), encoding="utf-8")
        writable.chmod(0o664)
        result = subprocess.run([str(VALIDATOR), str(writable), str(secure_output)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode == 0 or secure_output.exists():
            raise AssertionError("group-writable OCI config was accepted")

        oversized = directory / "oversized-source.json"
        oversized.write_bytes(json.dumps(BASE).encode("utf-8") + b" " * ((1 << 20) + 1))
        oversized.chmod(0o644)
        result = subprocess.run([str(VALIDATOR), str(oversized), str(secure_output)], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        if result.returncode == 0 or secure_output.exists():
            raise AssertionError("oversized OCI config with a valid prefix was accepted")

        bundle = directory / "bundle"
        (bundle / "rootfs").mkdir(parents=True)
        config = copy.deepcopy(BASE)
        config["hooks"] = {"prestart": []}
        bundle_config = bundle / "config.json"
        bundle_config.write_text(json.dumps(config), encoding="utf-8")
        bundle_config.chmod(0o644)
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
    print(
        f"runtime OCI fail-closed validation: PASS ({CASE_COUNT} semantic cases plus namespace, "
        "file-identity, and outer-cleanup boundaries)"
    )


if __name__ == "__main__":
    main()
