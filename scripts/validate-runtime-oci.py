#!/usr/bin/env python3
"""Fail-closed validation for the provisional Multikernel OCI subset."""

import json
import pathlib
import sys


TOP_LEVEL = {"ociVersion", "process", "root"}
PROCESS = {"terminal", "user", "args", "env", "cwd"}
USER = {"uid", "gid", "additionalGids"}
ROOT = {"path"}


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
                json.dumps(config, separators=(",", ":"), sort_keys=True) + "\n",
                encoding="utf-8",
            )
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"OCI configuration rejected: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
