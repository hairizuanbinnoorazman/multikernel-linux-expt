#!/usr/bin/env python3
"""Validate and resolve runtime-owned bootstrap artifacts from an approved manifest."""

import argparse
import hashlib
import json
import os
import pathlib
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
    initramfs = artifact(manifest["initramfs"], args.required_uid, args.skip_parent_safety)
    agent = artifact(manifest["agent"], args.required_uid, args.skip_parent_safety)
    relay = artifact(manifest["relay"], args.required_uid, args.skip_parent_safety)
    transport = manifest["transport"]
    exact_fields(transport, ["module", "module_name", "socket_option", "transport_id", "primary_role", "child_role"])
    expected_transport = {"module_name": "mk_transport", "socket_option": 9, "transport_id": 1, "primary_role": "server", "child_role": "client"}
    if {key: transport[key] for key in expected_transport} != expected_transport:
        raise ValueError("transport module, socket option, ID, or direction is incompatible")
    module = artifact(transport["module"], args.required_uid, args.skip_parent_safety)
    for extra in manifest.get("modules", []):
        artifact(extra, args.required_uid, args.skip_parent_safety)
    for executable in (kernel, agent, relay):
        x86_64_elf(executable)
    if not args.skip_modinfo:
        name = subprocess.check_output(["modinfo", "-F", "name", module], text=True).strip()
        vermagic = subprocess.check_output(["modinfo", "-F", "vermagic", module], text=True).strip()
        if name != "mk_transport" or not vermagic.startswith(manifest["kernel_release"] + " "):
            raise ValueError("transport module name or kernel release is incompatible")
    print(json.dumps({"kernel": kernel, "initramfs": initramfs, "agent": agent, "relay": relay, "module": module}, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print(f"bootstrap validation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
