#!/usr/bin/env python3
"""Fail-closed validation and guest projection for the Multikernel OCI subset.

Linux namespaces are consumed by the sandbox/CNI boundary rather than recreated
inside its dedicated child kernel. Annotations are inert metadata. Those two
fields are validated here and deliberately omitted from the guest projection;
all behavior-bearing unsupported fields remain fatal.
"""

import json
import pathlib
import sys


TOP_LEVEL = {"ociVersion", "process", "root", "linux", "annotations"}
PROCESS = {"terminal", "user", "args", "env", "cwd"}
USER = {"uid", "gid", "additionalGids"}
ROOT = {"path"}
LINUX = {"namespaces"}
NAMESPACE = {"type", "path"}
CHILD_BOUNDARY_NAMESPACES = {"pid", "ipc", "uts", "mount", "cgroup"}


def strict_object(pairs):
    value = {}
    for name, item in pairs:
        if name in value:
            raise ValueError(f"duplicate JSON object name {name!r}")
        value[name] = item
    return value


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


def validate(config):
    config = require_object(config, "config")
    reject_unknown(config, TOP_LEVEL, "OCI")
    if config.get("ociVersion") != "1.1.0":
        raise ValueError("ociVersion must be exactly 1.1.0")

    process = require_object(config.get("process"), "process")
    reject_unknown(process, PROCESS, "process")
    if "terminal" in process and not isinstance(process["terminal"], bool):
        raise ValueError("process.terminal must be boolean")
    args = process.get("args")
    if not isinstance(args, list) or not args or not all(isinstance(item, str) for item in args):
        raise ValueError("process.args must be a non-empty string array")
    env = process.get("env", [])
    if not isinstance(env, list) or not all(isinstance(item, str) for item in env):
        raise ValueError("process.env must be a string array")
    if not isinstance(process.get("cwd"), str) or not process["cwd"].startswith("/"):
        raise ValueError("process.cwd must be an absolute path")

    user = require_object(process.get("user"), "process.user")
    reject_unknown(user, USER, "process.user")
    require_uint32(user.get("uid"), "process.user.uid")
    require_uint32(user.get("gid"), "process.user.gid")
    gids = user.get("additionalGids", [])
    if not isinstance(gids, list):
        raise ValueError("process.user.additionalGids must be an array")
    for index, gid in enumerate(gids):
        require_uint32(gid, f"process.user.additionalGids[{index}]")

    root = require_object(config.get("root"), "root")
    reject_unknown(root, ROOT, "root")
    if not isinstance(root.get("path"), str) or not root["path"]:
        raise ValueError("root.path must be a non-empty string")

    validate_namespaces(config.get("linux"))
    annotations = config.get("annotations", {})
    if not isinstance(annotations, dict) or not all(isinstance(key, str) and isinstance(value, str) for key, value in annotations.items()):
        raise ValueError("annotations must be a string-to-string object")


def guest_projection(config):
    return {
        "ociVersion": config["ociVersion"],
        "process": config["process"],
        "root": config["root"],
    }


def main():
    if len(sys.argv) not in (2, 3):
        print("usage: validate-runtime-oci.py CONFIG [OUTPUT]", file=sys.stderr)
        return 2
    source = pathlib.Path(sys.argv[1])
    try:
        with source.open(encoding="utf-8") as stream:
            config = json.load(stream, object_pairs_hook=strict_object)
        validate(config)
        if len(sys.argv) == 3:
            destination = pathlib.Path(sys.argv[2])
            destination.write_text(
                json.dumps(guest_projection(config), separators=(",", ":"), sort_keys=True) + "\n",
                encoding="utf-8",
            )
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"OCI configuration rejected: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
