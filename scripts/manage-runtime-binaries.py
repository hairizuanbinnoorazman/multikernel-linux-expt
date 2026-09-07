#!/usr/bin/env python3
"""Inspect, install, activate, roll back, and uninstall runtime binaries."""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import stat
import sys


SCRIPT_DIR = Path(__file__).resolve().parent
MANIFEST_SCRIPT = SCRIPT_DIR / "runtime-release-manifest.py"
previous = sys.dont_write_bytecode
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("runtime_release_manifest_for_install", MANIFEST_SCRIPT)
release_manifest = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(release_manifest)
sys.dont_write_bytecode = previous

BASE = Path("/usr/local/lib/multikernel")
LINKS = {
    "containerd-shim-multikernel-v2": Path("/usr/local/bin/containerd-shim-multikernel-v2"),
    "mk-agentctl": Path("/usr/local/bin/mk-agentctl"),
    "mk-host-check": Path("/usr/local/bin/mk-host-check"),
    "mkruntimed": Path("/usr/local/sbin/mkruntimed"),
    "mknetd": Path("/usr/local/sbin/mknetd"),
    "mk-cni": Path("/opt/cni/bin/multikernel"),
}
RELEASE_ID = re.compile(r"^[0-9A-Za-z][0-9A-Za-z.+_-]{0,127}-[0-9a-f]{40}$")


def strict_json(path):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError(f"duplicate JSON key {key!r}")
            result[key] = value
        return result
    with path.open(encoding="utf-8") as source:
        return json.load(source, object_pairs_hook=pairs)


def rooted(root, absolute):
    return root / absolute.relative_to("/")


def release_id(manifest):
    value = f"{manifest['version']}-{manifest['revision']}"
    if not RELEASE_ID.fullmatch(value):
        raise ValueError("manifest does not produce a safe release identity")
    return value


def expected_target(link, component):
    current_binary = BASE / "current" / "bin" / component
    if str(link).startswith("/usr/local/"):
        return os.path.relpath(current_binary, link.parent)
    return str(current_binary)


def managed_link(path, expected):
    return path.is_symlink() and os.readlink(path) == expected


def preflight_links(root):
    current = rooted(root, BASE / "current")
    if os.path.lexists(current) and not current.is_symlink():
        raise ValueError(f"refuse non-symlink runtime activation path: {current}")
    for component, link in LINKS.items():
        path = rooted(root, link)
        expected = expected_target(link, component)
        if os.path.lexists(path) and not managed_link(path, expected):
            raise ValueError(f"refuse unrelated existing command path: {path}")


def require_directory(path, description):
    try:
        info = path.lstat()
    except FileNotFoundError:
        raise ValueError(f"missing {description}: {path}") from None
    if not path.is_dir() or path.is_symlink():
        raise ValueError(f"{description} must be a non-symlink directory: {path}")


def verify_release_input(manifest_path, binary_dir):
    require_directory(binary_dir, "release binary input")
    manifest_info = manifest_path.lstat()
    if not stat.S_ISREG(manifest_info.st_mode) or manifest_path.is_symlink():
        raise ValueError(f"release manifest must be a regular non-symlink file: {manifest_path}")
    manifest = strict_json(manifest_path)
    observed = release_manifest.create(binary_dir, manifest["version"], manifest["revision"])
    if manifest != observed:
        raise ValueError("manifest content differs from verified component identities and hashes")
    return manifest


def sync_directory(path):
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def atomic_symlink(target, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.parent / f".{path.name}.multikernel-{os.getpid()}"
    if os.path.lexists(temporary):
        raise ValueError(f"temporary activation path already exists: {temporary}")
    os.symlink(target, temporary)
    try:
        os.replace(temporary, path)
        sync_directory(path.parent)
    except BaseException:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass
        raise


def verify_installed(root, identifier):
    if not RELEASE_ID.fullmatch(identifier):
        raise ValueError("invalid release identity")
    directory = rooted(root, BASE / "releases" / identifier)
    require_directory(directory, "installed release")
    manifest = verify_release_input(directory / "release-manifest.json", directory / "bin")
    if release_id(manifest) != identifier:
        raise ValueError("installed release directory and manifest identity differ")
    return manifest


def activate(root, identifier):
    preflight_links(root)
    verify_installed(root, identifier)
    created = []
    try:
        for component, link in LINKS.items():
            path = rooted(root, link)
            expected = expected_target(link, component)
            if not managed_link(path, expected):
                atomic_symlink(expected, path)
                created.append(path)
        atomic_symlink(f"releases/{identifier}", rooted(root, BASE / "current"))
    except BaseException:
        for path in reversed(created):
            try:
                path.unlink()
                sync_directory(path.parent)
            except FileNotFoundError:
                pass
        raise


def install(root, manifest_path, binary_dir):
    preflight_links(root)
    manifest = verify_release_input(manifest_path, binary_dir)
    identifier = release_id(manifest)
    base = rooted(root, BASE)
    if os.path.lexists(base):
        require_directory(base, "runtime installation root")
    else:
        base.mkdir(parents=True, mode=0o755)
    releases = base / "releases"
    if os.path.lexists(releases):
        require_directory(releases, "runtime releases root")
    else:
        releases.mkdir(mode=0o755)
    destination = releases / identifier
    if os.path.lexists(destination):
        installed_manifest = verify_installed(root, identifier)
        if installed_manifest != manifest:
            raise ValueError(
                "release identity already exists with different manifest content"
            )
    else:
        staging = releases / f".{identifier}.staging-{os.getpid()}"
        staging.mkdir(mode=0o755)
        try:
            output_bin = staging / "bin"
            output_bin.mkdir(mode=0o755)
            for component in release_manifest.COMPONENTS:
                source = binary_dir / component
                output = output_bin / component
                descriptor = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o755)
                with source.open("rb") as input_stream, os.fdopen(descriptor, "wb") as output_stream:
                    shutil.copyfileobj(input_stream, output_stream)
                    output_stream.flush()
                    os.fsync(output_stream.fileno())
                os.chmod(output, 0o755)
            shutil.copyfile(manifest_path, staging / "release-manifest.json", follow_symlinks=False)
            os.chmod(staging / "release-manifest.json", 0o644)
            sync_directory(output_bin)
            sync_directory(staging)
            staged_manifest = verify_release_input(
                staging / "release-manifest.json", output_bin
            )
            if release_id(staged_manifest) != identifier:
                raise ValueError("staged release and manifest identity differ")
            os.rename(staging, destination)
            sync_directory(releases)
        except BaseException:
            shutil.rmtree(staging, ignore_errors=True)
            raise
        try:
            verify_installed(root, identifier)
        except BaseException:
            shutil.rmtree(destination, ignore_errors=True)
            sync_directory(releases)
            raise
    activate(root, identifier)
    return identifier


def inspection(root):
    base = rooted(root, BASE)
    current = base / "current"
    releases = base / "releases"
    if os.path.lexists(base):
        require_directory(base, "runtime installation root")
    if os.path.lexists(releases):
        require_directory(releases, "runtime releases root")
    return {
        "active": os.readlink(current) if current.is_symlink() else None,
        "releases": sorted(item.name for item in releases.iterdir() if item.is_dir() and not item.name.startswith(".")) if releases.is_dir() else [],
        "links": {
            str(link): {
                "present": os.path.lexists(rooted(root, link)),
                "managed": managed_link(rooted(root, link), expected_target(link, component)),
                "target": os.readlink(rooted(root, link)) if rooted(root, link).is_symlink() else None,
            }
            for component, link in LINKS.items()
        },
    }


def uninstall(root, apply):
    report = inspection(root)
    report["operation"] = "uninstall"
    report["applied"] = apply
    if not apply:
        return report
    preflight_links(root)
    for component, link in LINKS.items():
        path = rooted(root, link)
        if managed_link(path, expected_target(link, component)):
            path.unlink()
            sync_directory(path.parent)
    current = rooted(root, BASE / "current")
    if current.is_symlink():
        current.unlink()
        sync_directory(current.parent)
    report["preserved_releases"] = inspection(root)["releases"]
    return report


def remove_release(root, identifier, apply):
    manifest = verify_installed(root, identifier)
    current = rooted(root, BASE / "current")
    if current.is_symlink() and os.readlink(current) == f"releases/{identifier}":
        raise ValueError("refuse to remove the active release")
    report = {"operation": "remove-release", "release": identifier, "version": manifest["version"], "applied": apply}
    if apply:
        directory = rooted(root, BASE / "releases" / identifier)
        shutil.rmtree(directory)
        sync_directory(directory.parent)
    return report


def require_privilege(root, mutation):
    if mutation and root == Path("/") and os.geteuid() != 0:
        raise PermissionError("system mutation requires root")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path("/"), help=argparse.SUPPRESS)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("inspect")
    install_parser = commands.add_parser("install")
    install_parser.add_argument("manifest", type=Path)
    install_parser.add_argument("binary_dir", type=Path)
    activate_parser = commands.add_parser("activate")
    activate_parser.add_argument("release")
    uninstall_parser = commands.add_parser("uninstall")
    uninstall_parser.add_argument("--apply", action="store_true")
    remove_parser = commands.add_parser("remove-release")
    remove_parser.add_argument("release")
    remove_parser.add_argument("--apply", action="store_true")
    arguments = parser.parse_args()
    root = arguments.root.resolve(strict=True)
    mutation = arguments.command in ("install", "activate") or getattr(arguments, "apply", False)
    try:
        require_privilege(root, mutation)
        if arguments.command == "inspect":
            result = inspection(root)
        elif arguments.command == "install":
            result = {"installed_and_active": install(root, arguments.manifest, arguments.binary_dir)}
        elif arguments.command == "activate":
            activate(root, arguments.release)
            result = {"active": arguments.release}
        elif arguments.command == "uninstall":
            result = uninstall(root, arguments.apply)
        else:
            result = remove_release(root, arguments.release, arguments.apply)
        print(json.dumps(result, indent=2, sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"manage runtime binaries: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
