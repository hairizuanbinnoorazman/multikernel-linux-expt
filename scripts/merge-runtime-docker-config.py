#!/usr/bin/env python3
"""Add or remove the opt-in Multikernel shim without changing Docker defaults."""

import argparse
import json
from pathlib import Path
import sys


RUNTIME = "io.containerd.multikernel.v2"
ENTRY = {"runtimeType": RUNTIME}


class ConfigError(Exception):
    pass


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ConfigError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def load(path: Path) -> dict:
    if not path.exists():
        return {}
    try:
        value = json.loads(path.read_text(), object_pairs_hook=strict_object,
                           parse_constant=lambda value: (_ for _ in ()).throw(ConfigError(f"invalid number: {value}")))
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise ConfigError(f"invalid Docker configuration: {error}") from error
    if not isinstance(value, dict):
        raise ConfigError("Docker configuration must be a JSON object")
    return value


def merge(value: dict, remove: bool, enable_live_restore: bool = False) -> dict:
    value = dict(value)
    runtimes = value.get("runtimes", {})
    if not isinstance(runtimes, dict):
        raise ConfigError("Docker runtimes must be an object")
    runtimes = dict(runtimes)
    existing = runtimes.get(RUNTIME)
    if remove:
        if existing is not None and existing != ENTRY:
            raise ConfigError("refusing to remove a conflicting Multikernel runtime entry")
        runtimes.pop(RUNTIME, None)
    else:
        if existing is not None and existing != ENTRY:
            raise ConfigError("conflicting Multikernel runtime entry already exists")
        runtimes[RUNTIME] = dict(ENTRY)
    if runtimes:
        value["runtimes"] = runtimes
    else:
        value.pop("runtimes", None)
    # Deliberately do not add, remove, or rewrite default-runtime.
    if enable_live_restore:
        existing = value.get("live-restore")
        if existing is not None and existing is not True:
            raise ConfigError("conflicting Docker live-restore setting already exists")
        value["live-restore"] = True
    return value


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    operation = parser.add_mutually_exclusive_group()
    operation.add_argument("--remove", action="store_true")
    operation.add_argument("--enable-live-restore", action="store_true")
    args = parser.parse_args()
    try:
        result = merge(load(args.input), args.remove, args.enable_live_restore)
        with args.output.open("x", encoding="utf-8") as stream:
            stream.write(json.dumps(result, sort_keys=True, indent=2) + "\n")
    except (ConfigError, OSError) as error:
        print(f"Docker runtime merge failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
