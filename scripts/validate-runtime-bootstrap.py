#!/usr/bin/env python3
"""Validate and resolve runtime-owned bootstrap artifacts from an approved manifest."""

import argparse
import hashlib
import json
import os
import pathlib
import re
import stat
import subprocess
import sys


PINNED_MULTIKERNEL = "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7"
PINNED_KERF = "8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec"


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON field {key!r}")
        result[key] = value
    return result


def exact_fields(value, required, optional=()):
    if not isinstance(value, dict):
        raise ValueError("manifest object expected")
    fields = set(value)
    required = set(required)
    allowed = required | set(optional)
    if fields - allowed or required - fields:
        raise ValueError(f"manifest fields mismatch: missing={sorted(required-fields)} unknown={sorted(fields-allowed)}")


def secure_regular(path, digest, required_uid, skip_parent_safety=False):
    candidate = pathlib.Path(path)
    if not candidate.is_absolute():
        raise ValueError(f"artifact path is not absolute: {path}")
    if not skip_parent_safety:
        current = pathlib.Path("/")
        for component in candidate.parent.parts[1:]:
            current /= component
            info = current.lstat()
            if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
                raise ValueError(f"unsafe artifact parent: {current}")
            if info.st_uid not in (0, required_uid) or info.st_mode & 0o022:
                raise ValueError(f"artifact parent has unsafe owner or mode: {current}")
    info = candidate.lstat()
    if stat.S_ISLNK(info.st_mode) or not stat.S_ISREG(info.st_mode):
        raise ValueError(f"artifact is not a regular file: {path}")
    if info.st_uid != required_uid or info.st_mode & 0o022:
        raise ValueError(f"artifact has unsafe owner or mode: {path}")
    if not isinstance(digest, str) or len(digest) != 64:
        raise ValueError(f"artifact digest is malformed: {path}")
    actual = hashlib.sha256(candidate.read_bytes()).hexdigest()
    if actual != digest:
        raise ValueError(f"artifact digest mismatch: {path}")


def artifact(value, required_uid, skip_parent_safety=False):
    exact_fields(value, ["path", "sha256"])
    secure_regular(value["path"], value["sha256"], required_uid, skip_parent_safety)
    return value["path"]


def x86_64_elf(path):
    header = pathlib.Path(path).read_bytes()[:20]
    if len(header) < 20 or header[:5] != b"\x7fELF\x02" or int.from_bytes(header[18:20], "little") != 62:
        raise ValueError(f"artifact is not an x86-64 ELF image: {path}")


def kernel_config(kernel, release, required, required_uid):
    if not isinstance(release, str) or re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", release) is None:
        raise ValueError("kernel release is not a safe filename component")
    if not isinstance(required, list) or any(not isinstance(line, str) for line in required):
        raise ValueError("required kernel config must be a list of strings")
    # The kernel's parent hierarchy was already checked by artifact(). Hold a
    # no-follow descriptor so config verification cannot switch to a symlink.
    path = pathlib.Path(kernel).parent / ("config-" + release)
    descriptor = os.open(path, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        maximum = 16 << 20
        if not stat.S_ISREG(before.st_mode) or before.st_uid != required_uid or before.st_mode & 0o022 or not 0 < before.st_size <= maximum:
            raise ValueError("kernel config has unsafe type, owner, mode, or size")
        data = stream.read(maximum + 1)
        after = os.fstat(stream.fileno())
        identity = lambda info: (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
        if len(data) > maximum or identity(before) != identity(after):
            raise ValueError("kernel config changed during bounded verification")
    lines = {line.strip() for line in data.decode("utf-8").splitlines()}
    for line in required:
        if line not in lines:
            raise ValueError(f"kernel config is missing {line}")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest")
    parser.add_argument("expected_name")
    parser.add_argument("--required-uid", type=int, default=0)
    parser.add_argument("--skip-modinfo", action="store_true")
    parser.add_argument("--skip-parent-safety", action="store_true")
    args = parser.parse_args()
    manifest_path = pathlib.Path(args.manifest)
    secure_regular(manifest_path, hashlib.sha256(manifest_path.read_bytes()).hexdigest(), args.required_uid, args.skip_parent_safety)
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"), object_pairs_hook=strict_object)
    exact_fields(
        manifest,
        ["schema_version", "name", "architecture", "kernel_release", "compatibility", "kernel", "initramfs", "agent", "relay", "transport", "protocol", "required_config", "oci_features"],
        ["modules"],
    )
    if manifest["schema_version"] != 1 or manifest["name"] != args.expected_name or manifest["architecture"] != "amd64":
        raise ValueError("manifest identity or architecture is incompatible")
    compatibility = manifest["compatibility"]
    exact_fields(compatibility, ["multikernel_revision", "kerf_version", "kerf_revision"])
    if compatibility != {"multikernel_revision": PINNED_MULTIKERNEL, "kerf_version": "v0.2.0", "kerf_revision": PINNED_KERF}:
        raise ValueError("manifest compatibility pins are incompatible")
    protocol = manifest["protocol"]
    exact_fields(protocol, ["min", "max"])
    if protocol["min"] > 1 or protocol["max"] < 1:
        raise ValueError("manifest does not support protocol v1")

    kernel = artifact(manifest["kernel"], args.required_uid, args.skip_parent_safety)
    kernel_config(kernel, manifest["kernel_release"], manifest["required_config"], args.required_uid)
    initramfs = artifact(manifest["initramfs"], args.required_uid, args.skip_parent_safety)
    agent = artifact(manifest["agent"], args.required_uid, args.skip_parent_safety)
    relay = artifact(manifest["relay"], args.required_uid, args.skip_parent_safety)
    transport = manifest["transport"]
    exact_fields(transport, ["module", "module_name", "socket_option", "transport_id", "primary_role", "child_role"])
    expected_transport = {"module_name": "mk_transport", "socket_option": 9, "transport_id": 1, "primary_role": "server", "child_role": "client"}
    if {key: transport[key] for key in expected_transport} != expected_transport:
        raise ValueError("transport module, socket option, ID, or direction is incompatible")
    module = artifact(transport["module"], args.required_uid, args.skip_parent_safety)
    modules = [module] + [artifact(extra, args.required_uid, args.skip_parent_safety) for extra in manifest.get("modules", [])]
    for executable in (kernel, agent, relay):
        x86_64_elf(executable)
    if not args.skip_modinfo:
        for selected in modules:
            name = subprocess.check_output(["modinfo", "-F", "name", selected], text=True, timeout=10).strip()
            vermagic = subprocess.check_output(["modinfo", "-F", "vermagic", selected], text=True, timeout=10).strip()
            if (selected == module and name != "mk_transport") or not name or "\n" in name or "\n" in vermagic or not vermagic.startswith(manifest["kernel_release"] + " "):
                raise ValueError("module name or kernel release is incompatible")
    print(json.dumps({
        "manifest": str(manifest_path),
        "manifest_sha256": hashlib.sha256(manifest_path.read_bytes()).hexdigest(),
        "architecture": manifest["architecture"],
        "kernel_release": manifest["kernel_release"],
        "required_config": manifest["required_config"],
        "oci_features": manifest["oci_features"],
        "kernel": kernel,
        "initramfs": initramfs,
        "agent": agent,
        "relay": relay,
        "module": module,
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print(f"bootstrap validation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
