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
AGENT_INIT = REPO / "guest/mk-agent-init"
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
DOCKER_UNRESTRICTED_DEVICES = [
    {"allow": False, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 5, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 3, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 9, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 8, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 0, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 1, "access": "rwm"},
    {"allow": False, "type": "c", "major": 10, "minor": 229, "access": "rwm"},
    {"allow": False, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 5, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 3, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 9, "access": "rwm"},
    {"allow": True, "type": "c", "major": 1, "minor": 8, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 0, "access": "rwm"},
    {"allow": True, "type": "c", "major": 5, "minor": 1, "access": "rwm"},
    {"allow": False, "type": "c", "major": 10, "minor": 229, "access": "rwm"},
    {"allow": True, "type": "a", "major": -1, "minor": -1, "access": "rwm"},
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
        expected["process"] = {
            name: value for name, value in config["process"].items()
            if name not in {"apparmorProfile", "oomScoreAdj"}
        }
        expected["ociVersion"] = "1.1.0"
        if config.get("hostname"):
            expected["hostname"] = config["hostname"]
        policy = {name: config["linux"][name] for name in ("maskedPaths", "readonlyPaths") if name in config["linux"]}
        if policy:
            expected["linux"] = policy
        readonly_binds = [item for item in config.get("mounts", []) if item["destination"] not in {
            "/proc", "/dev", "/dev/pts", "/dev/shm", "/dev/mqueue", "/sys", "/sys/fs/cgroup", "/run",
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
        agent_init_text = AGENT_INIT.read_text(encoding="utf-8")
        if "mountpoint " in agent_init_text:
            raise AssertionError("agent init depends on the optional BusyBox mountpoint applet")
        if '/bin/busybox grep -qs " $1 " /proc/mounts' not in agent_init_text:
            raise AssertionError("agent init does not detect inherited mounts through controlled BusyBox")
        if 'is_mounted /dev || /bin/busybox mount -t devtmpfs' not in agent_init_text:
            raise AssertionError("agent init does not preserve an inherited devtmpfs mount")
        if 'install -m 0755 "$guest_init" "$root/init"' not in builder_text:
            raise AssertionError("runtime storage builder does not install the tested agent init")
        if '$source_manifest.after' in builder_text or 'mv "$source_manifest' in builder_text:
            raise AssertionError("outer builder reintroduced replacing source-manifest publication")
        if '"$source_manifest" --manifest-only' not in builder_text or 'cmp -s "$source_before_manifest" "$source_manifest"' not in builder_text:
            raise AssertionError("outer builder does not compare a no-replace final source manifest")
        if 'jq -n -e --slurpfile built' not in builder_text:
            raise AssertionError("outer builder final build/verify comparison lacks null input")
        if '/proc/$$/fd/255' not in builder_text or 'readlink -f "$script_descriptor"' not in builder_text:
            raise AssertionError("outer builder does not pin support assets to its open script generation")
        if 'script_dir=$(cd "$(dirname "$script_path")" && pwd -P)' not in builder_text:
            raise AssertionError("outer builder support directory does not derive from the pinned script")
        generation = directory / "generation" / "libexec"
        public = directory / "public"
        generation.mkdir(parents=True)
        public.mkdir()
        descriptor_probe = generation / "descriptor-probe.sh"
        descriptor_probe.write_text(
            "#!/usr/bin/env bash\n"
            "set -euo pipefail\n"
            "script_descriptor=/proc/$$/fd/255\n"
            "test -r \"$script_descriptor\"\n"
            "script_path=$(readlink -f \"$script_descriptor\")\n"
            "script_dir=$(cd \"$(dirname \"$script_path\")\" && pwd -P)\n"
            "printf '%s\\n' \"$script_dir\"\n",
            encoding="utf-8",
        )
        descriptor_probe.chmod(0o755)
        public_probe = public / "descriptor-probe.sh"
        public_probe.symlink_to(descriptor_probe)
        for command in ([str(public_probe)], ["bash", str(public_probe)]):
            resolved = subprocess.run(command, check=True, text=True, capture_output=True)
            if resolved.stdout.strip() != str(generation):
                raise AssertionError(f"open script descriptor resolved to {resolved.stdout!r}")
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
        docker_policy = copy.deepcopy(BASE)
        docker_policy["linux"].update({
            "maskedPaths": [
                "/proc/acpi", "/proc/asound", "/proc/interrupts", "/proc/kcore", "/proc/keys",
                "/proc/latency_stats", "/proc/sched_debug", "/proc/scsi", "/proc/timer_list",
                "/proc/timer_stats", "/sys/devices/virtual/powercap", "/sys/firmware",
            ],
            "readonlyPaths": ["/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"],
        })
        run_case(directory, "docker-root-path-policy", docker_policy, accepted=True)
        duplicate_policy = copy.deepcopy(docker_policy)
        duplicate_policy["linux"]["maskedPaths"].append("/proc/interrupts")
        run_case(directory, "docker-duplicate-masked-path", duplicate_policy)
        docker_cgroup_mount = copy.deepcopy(BASE)
        docker_cgroup_mount["mounts"] = [{
            "destination": "/sys/fs/cgroup", "type": "cgroup", "source": "cgroup",
            "options": ["ro", "nosuid", "noexec", "nodev"],
        }]
        run_case(directory, "docker-readonly-cgroup-mount", docker_cgroup_mount, accepted=True)
        modified_cgroup_mount = copy.deepcopy(docker_cgroup_mount)
        modified_cgroup_mount["mounts"][0]["options"][0] = "rw"
        run_case(directory, "docker-writable-cgroup-mount", modified_cgroup_mount)
        docker_shm_mount = copy.deepcopy(BASE)
        docker_shm_mount["mounts"] = [{
            "destination": "/dev/shm", "type": "tmpfs", "source": "shm",
            "options": ["nosuid", "noexec", "nodev", "mode=1777", "size=67108864"],
        }]
        run_case(directory, "docker-byte-sized-shm", docker_shm_mount, accepted=True)
        modified_shm_mount = copy.deepcopy(docker_shm_mount)
        modified_shm_mount["mounts"][0]["options"][-1] = "size=67108863"
        run_case(directory, "docker-wrong-sized-shm", modified_shm_mount)
        docker_readonly_bind = copy.deepcopy(BASE)
        docker_readonly_bind["mounts"] = [{
            "destination": "/etc/resolv.conf", "type": "bind",
            "source": "/var/lib/docker/containers/" + "a" * 64 + "/resolv.conf",
            "options": ["rbind", "rro", "rprivate"],
        }]
        run_case(directory, "docker-recursive-readonly-bind", docker_readonly_bind, accepted=True)
        docker_writable_bind = copy.deepcopy(docker_readonly_bind)
        docker_writable_bind["mounts"][0]["options"].remove("rro")
        run_case(directory, "docker-writable-managed-bind", docker_writable_bind)
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
        docker_id = "a" * 64
        docker_seed = copy.deepcopy(BASE)
        docker_seed["root"]["path"] = f"/var/lib/docker/rootfs/overlayfs/{docker_id}"
        docker_seed["mounts"] = [{
            "destination": f"/etc/{name}", "type": "bind",
            "source": f"/var/lib/docker/containers/{docker_id}/{name}",
            "options": ["rbind", "rprivate"],
        } for name in ("resolv.conf", "hostname", "hosts")]
        seed_source = directory / "docker-private-seed-source.json"
        seed_guest = directory / "docker-private-seed-guest.json"
        seed_plan = directory / "docker-private-seed-plan.json"
        seed_source.write_text(json.dumps(docker_seed), encoding="utf-8")
        seed_source.chmod(0o644)
        seeded = subprocess.run(
            [str(VALIDATOR), str(seed_source), str(seed_guest), str(seed_plan)],
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=False,
        )
        if seeded.returncode != 0:
            raise AssertionError(f"Docker private seed projection failed: {seeded.stderr!r}")
        if "mounts" in json.loads(seed_guest.read_text(encoding="utf-8")):
            raise AssertionError("Docker private seeds crossed into the guest as host binds")
        seed_records = json.loads(seed_plan.read_text(encoding="utf-8"))["readonly_binds"]
        if len(seed_records) != 3 or any(
                item["options"] != ["bind", "rw", "nodev", "nosuid", "noexec"]
                for item in seed_records):
            raise AssertionError("Docker private seeds were not canonicalized")
        for name, mutate in (
            ("docker-private-seed-wrong-root", lambda config: config["root"].update(path="rootfs")),
            ("docker-private-seed-wrong-id", lambda config: config["mounts"][0].update(
                source=f"/var/lib/docker/containers/{'b' * 64}/resolv.conf")),
            ("docker-private-seed-wrong-destination", lambda config: config["mounts"][0].update(
                destination="/etc/passwd")),
            ("docker-private-seed-extra-option", lambda config: config["mounts"][0]["options"].append("rw")),
        ):
            rejected_seed = copy.deepcopy(docker_seed)
            mutate(rejected_seed)
            run_case(directory, name, rejected_seed)
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
            "apparmorProfile", "oomScoreAdj", "scheduler", "selinuxLabel",
            "ioPriority", "commandLine",
        ]
        for field in process_fields:
            config = copy.deepcopy(BASE)
            config["process"][field] = False if field == "noNewPrivileges" else {}
            run_case(directory, f"process-{field}", config)
        console_size = copy.deepcopy(BASE)
        console_size["process"].update(terminal=True, consoleSize={"width": 91, "height": 37})
        run_case(directory, "terminal-console-size", console_size, accepted=True)
        for name, value, terminal in (
            ("console-size-without-terminal", {"width": 91, "height": 37}, False),
            ("console-size-missing-height", {"width": 91}, True),
            ("console-size-oversized-width", {"width": 65536, "height": 37}, True),
            ("console-size-boolean-height", {"width": 91, "height": True}, True),
            ("console-size-unknown-field", {"width": 91, "height": 37, "depth": 1}, True),
        ):
            config = copy.deepcopy(BASE)
            config["process"].update(terminal=terminal, consoleSize=value)
            run_case(directory, name, config)
        docker_unconfined = copy.deepcopy(BASE)
        docker_unconfined["process"].update(apparmorProfile="unconfined", oomScoreAdj=0)
        docker_unconfined["linux"]["sysctl"] = {
            "net.ipv4.ip_unprivileged_port_start": "1024",
            "net.ipv4.ping_group_range": "1 0",
        }
        docker_unconfined["linux"]["resources"] = {
            "devices": copy.deepcopy(DOCKER_UNRESTRICTED_DEVICES),
            "blockIO": {},
        }
        docker_unconfined["linux"]["cgroupsPath"] = "system.slice:docker:" + "a" * 64
        run_case(directory, "docker-explicit-unconfined", docker_unconfined, accepted=True)
        for name, field, value in (
            ("docker-default-apparmor", "apparmorProfile", "docker-default"),
            ("nonzero-oom-adjustment", "oomScoreAdj", 1),
            ("boolean-oom-adjustment", "oomScoreAdj", False),
            ("floating-oom-adjustment", "oomScoreAdj", 0.0),
        ):
            config = copy.deepcopy(BASE)
            config["process"][field] = value
            run_case(directory, name, config)
        for name, sysctl in (
            ("docker-permissive-sysctls", {
                "net.ipv4.ip_unprivileged_port_start": "0",
                "net.ipv4.ping_group_range": "0 2147483647",
            }),
            ("partial-child-default-sysctls", {
                "net.ipv4.ip_unprivileged_port_start": "1024",
            }),
            ("extra-child-default-sysctl", {
                "net.ipv4.ip_unprivileged_port_start": "1024",
                "net.ipv4.ping_group_range": "1 0",
                "kernel.hostname": "guest",
            }),
            ("typed-child-default-sysctl", {
                "net.ipv4.ip_unprivileged_port_start": 1024,
                "net.ipv4.ping_group_range": "1 0",
            }),
            ("nonobject-sysctl", []),
        ):
            config = copy.deepcopy(BASE)
            config["linux"]["sysctl"] = sysctl
            run_case(directory, name, config)
        for name, mutate in (
            ("docker-restrictive-devices", lambda resources: resources["devices"].pop()),
            ("docker-modified-terminal-device", lambda resources: resources["devices"][-1].update(allow=False)),
            ("docker-nonempty-blockio", lambda resources: resources["blockIO"].update(weight=100)),
        ):
            config = copy.deepcopy(docker_unconfined)
            mutate(config["linux"]["resources"])
            run_case(directory, name, config)
        for name, path in (
            ("docker-wrong-systemd-slice", "user.slice:docker:" + "a" * 64),
            ("docker-short-systemd-id", "system.slice:docker:" + "a" * 63),
            ("docker-uppercase-systemd-id", "system.slice:docker:" + "A" * 64),
        ):
            config = copy.deepcopy(docker_unconfined)
            config["linux"]["cgroupsPath"] = path
            run_case(directory, name, config)
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
