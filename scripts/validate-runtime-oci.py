#!/usr/bin/env python3
"""Fail-closed validation and guest projection for the Multikernel OCI subset.

Linux namespaces are consumed by the sandbox/CNI boundary rather than recreated
inside its dedicated child kernel. Annotations are inert metadata. Those two
fields are validated here and deliberately omitted from the guest projection;
all behavior-bearing unsupported fields remain fatal.
"""

import json
import os
import pathlib
import re
import stat
import sys


TOP_LEVEL = {"ociVersion", "process", "root", "linux", "annotations", "mounts", "hostname"}
PROCESS = {"terminal", "user", "args", "env", "cwd", "noNewPrivileges", "rlimits", "capabilities"}
USER = {"uid", "gid", "additionalGids"}
CAPABILITY_SETS = {"bounding", "effective", "inheritable", "permitted", "ambient"}
CAPABILITIES = {
    "CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_DAC_READ_SEARCH", "CAP_FOWNER", "CAP_FSETID", "CAP_KILL",
    "CAP_SETGID", "CAP_SETUID", "CAP_SETPCAP", "CAP_LINUX_IMMUTABLE", "CAP_NET_BIND_SERVICE",
    "CAP_NET_BROADCAST", "CAP_NET_ADMIN", "CAP_NET_RAW", "CAP_IPC_LOCK", "CAP_IPC_OWNER",
    "CAP_SYS_MODULE", "CAP_SYS_RAWIO", "CAP_SYS_CHROOT", "CAP_SYS_PTRACE", "CAP_SYS_PACCT",
    "CAP_SYS_ADMIN", "CAP_SYS_BOOT", "CAP_SYS_NICE", "CAP_SYS_RESOURCE", "CAP_SYS_TIME",
    "CAP_SYS_TTY_CONFIG", "CAP_MKNOD", "CAP_LEASE", "CAP_AUDIT_WRITE", "CAP_AUDIT_CONTROL",
    "CAP_SETFCAP", "CAP_MAC_OVERRIDE", "CAP_MAC_ADMIN", "CAP_SYSLOG", "CAP_WAKE_ALARM",
    "CAP_BLOCK_SUSPEND", "CAP_AUDIT_READ", "CAP_PERFMON", "CAP_BPF", "CAP_CHECKPOINT_RESTORE",
}
RLIMITS = {
    "RLIMIT_AS", "RLIMIT_CORE", "RLIMIT_CPU", "RLIMIT_DATA", "RLIMIT_FSIZE", "RLIMIT_LOCKS",
    "RLIMIT_MEMLOCK", "RLIMIT_MSGQUEUE", "RLIMIT_NICE", "RLIMIT_NOFILE", "RLIMIT_NPROC",
    "RLIMIT_RSS", "RLIMIT_RTPRIO", "RLIMIT_RTTIME", "RLIMIT_SIGPENDING", "RLIMIT_STACK",
}
ROOT = {"path", "readonly"}
LINUX = {"namespaces", "resources", "cgroupsPath", "maskedPaths", "readonlyPaths"}
NAMESPACE = {"type", "path"}
CHILD_BOUNDARY_NAMESPACES = {"pid", "ipc", "uts", "mount", "cgroup"}
SAFE_MASKED_PATHS = {
    "/proc/acpi", "/proc/asound", "/proc/kcore", "/proc/keys", "/proc/latency_stats",
    "/proc/timer_list", "/proc/timer_stats", "/proc/sched_debug", "/sys/firmware",
    "/sys/devices/virtual/powercap", "/proc/scsi",
}
SAFE_READONLY_PATHS = {"/proc/bus", "/proc/fs", "/proc/irq", "/proc/sys", "/proc/sysrq-trigger"}
MAX_PROCESS_ARGS = 256
MAX_PROCESS_ENV = 1024
MAX_PROCESS_TEXT = 128 << 10
MAX_SUPPLEMENTAL_GIDS = 256
MAX_CONFIG_BYTES = 1 << 20
DEFAULT_MOUNTS = {
    "/proc": ("proc", "proc", {"nosuid", "noexec", "nodev"}),
    "/dev": ("tmpfs", "tmpfs", {"nosuid", "strictatime", "mode=755", "size=65536k"}),
    "/dev/pts": ("devpts", "devpts", {"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620", "gid=5"}),
    "/dev/shm": ("tmpfs", "shm", {"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"}),
    "/dev/mqueue": ("mqueue", "mqueue", {"nosuid", "noexec", "nodev"}),
    "/sys": ("sysfs", "sysfs", {"nosuid", "noexec", "nodev", "ro"}),
    "/run": ("tmpfs", "tmpfs", {"nosuid", "strictatime", "mode=755", "size=65536k"}),
}
READONLY_BIND_KINDS = {"bind", "rbind"}
READONLY_BIND_OPTIONAL = {"private", "rprivate", "nodev", "nosuid", "noexec", "relatime", "noatime", "strictatime"}
SANITIZED_READONLY_BIND_OPTIONS = {"bind", "ro", "nodev", "nosuid", "noexec"}
PROTECTED_BIND_DESTINATIONS = ("/dev", "/proc", "/run", "/sys")
MAX_READONLY_BINDS = 8
INHERITED_CONFIG = re.compile(r"/proc/self/fd/([0-9]+)/config\.json")


def strict_object(pairs):
    value = {}
    for name, item in pairs:
        if name in value:
            raise ValueError(f"duplicate JSON object name {name!r}")
        value[name] = item
    return value


def load_config(source):
    if not source.is_absolute() or pathlib.Path(os.path.normpath(source)) != source:
        raise ValueError("OCI config path must be absolute and canonical")
    inherited = INHERITED_CONFIG.fullmatch(os.fspath(source))
    if inherited:
        directory = int(inherited.group(1))
        info = os.fstat(directory)
        if (not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or
                info.st_mode & 0o022):
            raise ValueError("inherited OCI bundle must be a caller-owned non-writable directory")
        descriptor = os.open(
            "config.json",
            os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=directory,
        )
        return load_config_descriptor(descriptor)
    directory = os.open("/", os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    descriptor = None
    try:
        for component in source.parts[1:-1]:
            child = os.open(
                component,
                os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC | os.O_NOFOLLOW,
                dir_fd=directory,
            )
            os.close(directory)
            directory = child
        descriptor = os.open(
            source.name,
            os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW,
            dir_fd=directory,
        )
    finally:
        os.close(directory)

    return load_config_descriptor(descriptor)


def load_config_descriptor(descriptor):
    try:
        before = os.fstat(descriptor)
        if (not stat.S_ISREG(before.st_mode) or before.st_nlink != 1 or
                before.st_uid != os.geteuid() or before.st_mode & 0o022 or
                before.st_size > MAX_CONFIG_BYTES):
            raise ValueError("OCI config must be a bounded private caller-owned single-link regular file")
        chunks = []
        retained = 0
        while retained <= MAX_CONFIG_BYTES:
            chunk = os.read(descriptor, min(65536, MAX_CONFIG_BYTES + 1 - retained))
            if not chunk:
                break
            chunks.append(chunk)
            retained += len(chunk)
        if retained > MAX_CONFIG_BYTES:
            raise ValueError("OCI config exceeds the retained byte limit")
        after = os.fstat(descriptor)
        if ((before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns, before.st_ctime_ns) !=
                (after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns)):
            raise ValueError("OCI config identity changed while reading")
        return json.loads(b"".join(chunks).decode("utf-8"), object_pairs_hook=strict_object)
    finally:
        os.close(descriptor)


def require_object(value, name):
    if not isinstance(value, dict):
        raise ValueError(f"{name} must be an object")
    return value


def reject_unknown(value, allowed, name):
    unknown = sorted(set(value) - allowed)
    if unknown:
        raise ValueError(f"unsupported {name} field(s): {', '.join(unknown)}")


def require_uint32(value, name):
    if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 0xFFFFFFFF:
        raise ValueError(f"{name} must be an unsigned 32-bit integer")


def validate_namespaces(value):
    linux = require_object(value, "linux")
    reject_unknown(linux, LINUX, "linux")
    namespaces = linux.get("namespaces")
    if not isinstance(namespaces, list):
        raise ValueError("linux.namespaces must be an array")
    seen = set()
    for index, item in enumerate(namespaces):
        item = require_object(item, f"linux.namespaces[{index}]")
        reject_unknown(item, NAMESPACE, f"linux.namespaces[{index}]")
        kind = item.get("type")
        path = item.get("path", "")
        if kind in seen:
            raise ValueError(f"duplicate Linux namespace type {kind!r}")
        seen.add(kind)
        if kind == "network":
            if not isinstance(path, str):
                raise ValueError("network namespace path must be a string")
            if path and (not path.startswith("/") or pathlib.PurePosixPath(path).as_posix() != path or ".." in pathlib.PurePosixPath(path).parts):
                raise ValueError("network namespace path must be absolute and canonical")
        elif kind in CHILD_BOUNDARY_NAMESPACES:
            if path not in (None, ""):
                raise ValueError(f"joining an existing {kind} namespace is unsupported")
        else:
            raise ValueError(f"unsupported Linux namespace type {kind!r}")
    if "network" not in seen:
        raise ValueError("exactly one Linux network namespace is required")


def canonical_absolute_path(value, name):
    if (not isinstance(value, str) or not value.startswith("/") or "\0" in value or
            len(value) > 4096 or pathlib.PurePosixPath(value).as_posix() != value or
            ".." in pathlib.PurePosixPath(value).parts):
        raise ValueError(f"{name} must be an absolute canonical bounded path")


def validate_mounts(value):
    if not isinstance(value, list):
        raise ValueError("mounts must be an array")
    seen_defaults = set()
    bind_destinations = []
    for index, item in enumerate(value):
        item = require_object(item, f"mounts[{index}]")
        reject_unknown(item, {"destination", "type", "source", "options"}, f"mounts[{index}]")
        destination = item.get("destination")
        actual_options = item.get("options", [])
        if destination in DEFAULT_MOUNTS:
            if destination in seen_defaults:
                raise ValueError(f"unsupported or duplicate OCI mount destination {destination!r}")
            seen_defaults.add(destination)
            mount_type, source, options = DEFAULT_MOUNTS[destination]
            if (item.get("type") != mount_type or item.get("source") != source or
                    not isinstance(actual_options, list) or len(actual_options) != len(set(actual_options)) or
                    set(actual_options) != options):
                raise ValueError(f"OCI mount {destination!r} differs from the enforced default contract")
            continue
        canonical_absolute_path(destination, f"mounts[{index}].destination")
        source = item.get("source")
        canonical_absolute_path(source, f"mounts[{index}].source")
        if source == "/":
            raise ValueError("read-only bind source may not be the host root")
        if item.get("type") != "bind":
            raise ValueError(f"unsupported OCI mount type at {destination!r}")
        option_set = set(actual_options) if isinstance(actual_options, list) and all(
            isinstance(option, str) for option in actual_options
        ) else set()
        if (not isinstance(actual_options, list) or
                not all(isinstance(option, str) for option in actual_options) or
                len(actual_options) != len(option_set) or "ro" not in option_set or
                len(option_set & READONLY_BIND_KINDS) != 1 or
                not option_set <= ({"ro"} | READONLY_BIND_KINDS | READONLY_BIND_OPTIONAL) or
                len(option_set & {"private", "rprivate"}) > 1):
            raise ValueError(f"read-only bind {destination!r} differs from the enforced option contract")
        if destination == "/" or any(destination == path or destination.startswith(path + "/")
                                     for path in PROTECTED_BIND_DESTINATIONS):
            raise ValueError(f"read-only bind destination {destination!r} overlaps a runtime-owned path")
        if any(destination == prior or destination.startswith(prior + "/") or prior.startswith(destination + "/")
               for prior in bind_destinations):
            raise ValueError("read-only bind destinations must be unique and non-overlapping")
        bind_destinations.append(destination)
    if len(bind_destinations) > MAX_READONLY_BINDS:
        raise ValueError(f"at most {MAX_READONLY_BINDS} read-only bind inputs are supported")
    return [item for item in value if item.get("destination") not in DEFAULT_MOUNTS]


def validate_path_policy(values, allowed, name):
    if not isinstance(values, list) or not all(isinstance(item, str) for item in values):
        raise ValueError(f"linux.{name} must be a string array")
    if len(values) != len(set(values)) or any(item not in allowed for item in values):
        raise ValueError(f"linux.{name} contains a duplicate or unsupported path")


def validate(config):
    config = require_object(config, "config")
    reject_unknown(config, TOP_LEVEL, "OCI")
    version = config.get("ociVersion")
    if not isinstance(version, str) or not re.fullmatch(r"1\.(?:0|1|2|3)\.\d+(?:-[A-Za-z0-9.-]+)?", version):
        raise ValueError("ociVersion must be a supported OCI 1.0-1.3 version")

    process = require_object(config.get("process"), "process")
    reject_unknown(process, PROCESS, "process")
    if "terminal" in process and not isinstance(process["terminal"], bool):
        raise ValueError("process.terminal must be boolean")
    args = process.get("args")
    if (not isinstance(args, list) or not args or len(args) > MAX_PROCESS_ARGS or
            not all(isinstance(item, str) and "\0" not in item for item in args) or not args[0]):
        raise ValueError("process.args must contain a bounded non-empty argv[0] without NUL")
    env = process.get("env", [])
    if not isinstance(env, list) or len(env) > MAX_PROCESS_ENV or not all(isinstance(item, str) and "\0" not in item for item in env):
        raise ValueError("process.env must be a bounded string array without NUL")
    environment_names = [item.partition("=")[0] for item in env]
    if any("=" not in item or not name for item, name in zip(env, environment_names)) or len(environment_names) != len(set(environment_names)):
        raise ValueError("process.env entries must have unique non-empty names")
    if sum(len(item) + 1 for item in args + env) > MAX_PROCESS_TEXT:
        raise ValueError("process args and environment exceed the retained byte limit")
    cwd = process.get("cwd")
    cwd_path = pathlib.PurePosixPath(cwd) if isinstance(cwd, str) else None
    if (not isinstance(cwd, str) or not cwd.startswith("/") or "\0" in cwd or
            len(cwd) > 4096 or cwd_path.as_posix() != cwd or ".." in cwd_path.parts):
        raise ValueError("process.cwd must be absolute, canonical, and bounded")
    if "noNewPrivileges" in process and not isinstance(process["noNewPrivileges"], bool):
        raise ValueError("process.noNewPrivileges must be boolean")
    rlimits = process.get("rlimits", [])
    if not isinstance(rlimits, list) or len(rlimits) > 32:
        raise ValueError("process.rlimits must be an array of at most 32 limits")
    seen_limits = set()
    for index, limit in enumerate(rlimits):
        limit = require_object(limit, f"process.rlimits[{index}]")
        reject_unknown(limit, {"type", "hard", "soft"}, f"process.rlimits[{index}]")
        kind = limit.get("type")
        if kind not in RLIMITS or kind in seen_limits:
            raise ValueError(f"unsupported or duplicate process rlimit {kind!r}")
        seen_limits.add(kind)
        for field in ("hard", "soft"):
            value = limit.get(field)
            if isinstance(value, bool) or not isinstance(value, int) or not 0 <= value <= 0xFFFFFFFFFFFFFFFF:
                raise ValueError(f"process.rlimits[{index}].{field} must be uint64")
        if limit["soft"] > limit["hard"]:
            raise ValueError(f"process.rlimits[{index}] soft limit exceeds hard limit")
    capabilities = process.get("capabilities")
    if capabilities is not None:
        capabilities = require_object(capabilities, "process.capabilities")
        reject_unknown(capabilities, CAPABILITY_SETS, "process.capabilities")
        for name, values in capabilities.items():
            if not isinstance(values, list) or len(values) > len(CAPABILITIES) or not all(isinstance(item, str) for item in values):
                raise ValueError(f"process.capabilities.{name} must be a bounded string array")
            if len(values) != len(set(values)) or any(item not in CAPABILITIES for item in values):
                raise ValueError(f"process.capabilities.{name} contains an unknown or duplicate capability")
        if process.get("user", {}).get("uid") != 0:
            raise ValueError("non-root processes may not provide a Linux capability contract")
        bounding = set(capabilities.get("bounding", []))
        permitted = set(capabilities.get("permitted", []))
        effective = set(capabilities.get("effective", []))
        inheritable = set(capabilities.get("inheritable", []))
        ambient = set(capabilities.get("ambient", []))
        if not permitted <= bounding or not effective <= permitted or not ambient <= (permitted & inheritable):
            raise ValueError("process capability sets violate containment")

    user = require_object(process.get("user"), "process.user")
    reject_unknown(user, USER, "process.user")
    require_uint32(user.get("uid"), "process.user.uid")
    require_uint32(user.get("gid"), "process.user.gid")
    gids = user.get("additionalGids", [])
    if not isinstance(gids, list) or len(gids) > MAX_SUPPLEMENTAL_GIDS:
        raise ValueError("process.user.additionalGids must be a bounded array")
    seen_gids = set()
    for index, gid in enumerate(gids):
        require_uint32(gid, f"process.user.additionalGids[{index}]")
        if gid in seen_gids:
            raise ValueError("process.user.additionalGids must contain unique values")
        seen_gids.add(gid)

    root = require_object(config.get("root"), "root")
    reject_unknown(root, ROOT, "root")
    if not isinstance(root.get("path"), str) or not root["path"]:
        raise ValueError("root.path must be a non-empty string")
    if "readonly" in root and not isinstance(root["readonly"], bool):
        raise ValueError("root.readonly must be boolean")

    linux = config.get("linux")
    validate_namespaces(linux)
    if "resources" in linux:
        if linux["resources"] != {"devices": [{"allow": False, "access": "rwm"}]}:
            raise ValueError("only the default deny-all device resource contract is supported")
    if "cgroupsPath" in linux:
        path = linux["cgroupsPath"]
        if (not isinstance(path, str) or not path.startswith("/") or
                pathlib.PurePosixPath(path).as_posix() != path or
                ".." in pathlib.PurePosixPath(path).parts or len(path) > 4096):
            raise ValueError("linux.cgroupsPath must be absolute, canonical, and bounded")
    validate_path_policy(linux.get("maskedPaths", []), SAFE_MASKED_PATHS, "maskedPaths")
    validate_path_policy(linux.get("readonlyPaths", []), SAFE_READONLY_PATHS, "readonlyPaths")
    validate_mounts(config.get("mounts", []))
    hostname = config.get("hostname", "")
    if not isinstance(hostname, str) or len(hostname) > 63 or (hostname and not re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?", hostname)):
        raise ValueError("hostname must be an RFC1123-compatible value of at most 63 bytes")
    annotations = config.get("annotations", {})
    if not isinstance(annotations, dict) or not all(isinstance(key, str) and isinstance(value, str) for key, value in annotations.items()):
        raise ValueError("annotations must be a string-to-string object")


def guest_projection(config):
    result = {
        "ociVersion": "1.1.0",
        "process": config["process"],
        "root": config["root"],
    }
    if config.get("hostname"):
        result["hostname"] = config["hostname"]
    linux = config["linux"]
    policy = {name: linux[name] for name in ("maskedPaths", "readonlyPaths") if name in linux}
    if policy:
        result["linux"] = policy
    readonly_binds = validate_mounts(config.get("mounts", []))
    if readonly_binds:
        result["mounts"] = [
            {
                "destination": item["destination"],
                "type": "bind",
                "source": item["destination"],
                "options": sorted(SANITIZED_READONLY_BIND_OPTIONS),
            }
            for item in readonly_binds
        ]
    return result


def bind_projection(config):
    return {
        "schema_version": 1,
        "readonly_binds": [
            {
                "destination": item["destination"],
                "type": "bind",
                "source": item["source"],
                "options": ["bind", "ro", "nodev", "nosuid", "noexec"],
            }
            for item in validate_mounts(config.get("mounts", []))
        ],
    }


def main():
    if len(sys.argv) not in (2, 3, 4):
        print("usage: validate-runtime-oci.py CONFIG [OUTPUT [BIND-OUTPUT]]", file=sys.stderr)
        return 2
    source = pathlib.Path(sys.argv[1])
    try:
        config = load_config(source)
        validate(config)
        if len(sys.argv) == 3:
            destination = pathlib.Path(sys.argv[2])
            destination.write_text(
                json.dumps(guest_projection(config), separators=(",", ":"), sort_keys=True) + "\n",
                encoding="utf-8",
            )
        elif len(sys.argv) == 4:
            pathlib.Path(sys.argv[2]).write_text(
                json.dumps(guest_projection(config), separators=(",", ":"), sort_keys=True) + "\n",
                encoding="utf-8",
            )
            pathlib.Path(sys.argv[3]).write_text(
                json.dumps(bind_projection(config), separators=(",", ":"), sort_keys=True) + "\n",
                encoding="utf-8",
            )
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"OCI configuration rejected: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
