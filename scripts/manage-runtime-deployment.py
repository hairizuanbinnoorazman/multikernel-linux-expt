#!/usr/bin/env python3
"""Install and atomically select immutable Multikernel service/config sets."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import sys


REPO = Path(__file__).resolve().parent.parent
BASE = Path("/etc/multikernel")
ASSETS = {
    "systemd/sys-fs-multikernel.mount": REPO / "deploy/systemd/sys-fs-multikernel.mount",
    "systemd/mkruntimed.service": REPO / "deploy/systemd/mkruntimed.service",
    "systemd/mknetd.service": REPO / "deploy/systemd/mknetd.service",
    "cni/10-multikernel.conf": REPO / "deploy/cni/10-multikernel.conf",
    "containerd/20-multikernel-runtime.toml": REPO / "deploy/containerd/20-multikernel-runtime.toml",
    "libexec/build-runtime-container-initramfs.sh": REPO / "scripts/build-runtime-container-initramfs.sh",
    "libexec/validate-runtime-oci.py": REPO / "scripts/validate-runtime-oci.py",
    "libexec/validate-runtime-bootstrap.py": REPO / "scripts/validate-runtime-bootstrap.py",
    "libexec/validate-runtime-root.py": REPO / "scripts/validate-runtime-root.py",
    "libexec/validate-runtime-image.py": REPO / "scripts/validate-runtime-image.py",
    "libexec/build-runtime-rootfs.py": REPO / "scripts/build-runtime-rootfs.py",
    "libexec/runtime-storage-identity.py": REPO / "scripts/runtime-storage-identity.py",
    "libexec/build-runtime-storage.py": REPO / "scripts/build-runtime-storage.py",
    "libexec/verify-runtime-rootfs.py": REPO / "scripts/verify-runtime-rootfs.py",
    "libexec/guest/mk-agent-init": REPO / "guest/mk-agent-init",
    "libexec/guest/runtime-mediated-init": REPO / "guest/runtime-mediated-init",
}
LINKS = {
    Path("/etc/multikernel/runtime.env"): "runtime.env",
    Path("/etc/mkruntime/config.json"): "mkruntime/config.json",
    Path("/etc/systemd/system/sys-fs-multikernel.mount"): "systemd/sys-fs-multikernel.mount",
    Path("/etc/systemd/system/mkruntimed.service"): "systemd/mkruntimed.service",
    Path("/etc/systemd/system/mknetd.service"): "systemd/mknetd.service",
    Path("/etc/cni/net.d/10-multikernel.conf"): "cni/10-multikernel.conf",
    Path("/etc/containerd/conf.d/20-multikernel-runtime.toml"): "containerd/20-multikernel-runtime.toml",
    Path("/usr/local/libexec/multikernel/build-runtime-container-initramfs.sh"): "libexec/build-runtime-container-initramfs.sh",
    Path("/usr/local/libexec/multikernel/validate-runtime-oci.py"): "libexec/validate-runtime-oci.py",
    Path("/usr/local/libexec/multikernel/validate-runtime-bootstrap.py"): "libexec/validate-runtime-bootstrap.py",
    Path("/usr/local/libexec/multikernel/validate-runtime-root.py"): "libexec/validate-runtime-root.py",
    Path("/usr/local/libexec/multikernel/validate-runtime-image.py"): "libexec/validate-runtime-image.py",
    Path("/usr/local/libexec/multikernel/build-runtime-rootfs.py"): "libexec/build-runtime-rootfs.py",
    Path("/usr/local/libexec/multikernel/runtime-storage-identity.py"): "libexec/runtime-storage-identity.py",
    Path("/usr/local/libexec/multikernel/build-runtime-storage.py"): "libexec/build-runtime-storage.py",
    Path("/usr/local/libexec/multikernel/verify-runtime-rootfs.py"): "libexec/verify-runtime-rootfs.py",
    Path("/usr/local/libexec/multikernel/guest/mk-agent-init"): "libexec/guest/mk-agent-init",
    Path("/usr/local/libexec/multikernel/guest/runtime-mediated-init"): "libexec/guest/runtime-mediated-init",
}
EXECUTABLES = {name for name in ASSETS if name.startswith("libexec/")}
ENV_KEYS = {
    "MKRUNTIME_POOL_CPUS", "MKRUNTIME_POOL_MEMORY", "MKRUNTIME_POOL_MEMORY_RESERVE",
    "MKRUNTIME_CMDLINE", "MKNETWORK_SUBNET", "MKNETWORK_EGRESS", "MKNETWORK_MTU",
    "MKNETWORK_DNS",
}
HOST_FIELDS = {
    "schema_version", "state_directory", "socket_path", "kerf_executable",
    "multikernel_sysfs_root", "kernel_manifest_directory", "min_primary_cpus",
    "min_primary_memory_bytes", "forbidden_apic_ids", "backend_timeout_seconds",
    "max_frame_size_bytes",
}


def rooted(root: Path, absolute: Path) -> Path:
    return root / absolute.relative_to("/")


def strict_json_bytes(data: bytes):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError(f"duplicate JSON key {key!r}")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=pairs)


def secure_input(path: Path, limit: int = 1 << 20) -> bytes:
    info = path.lstat()
    if (not stat.S_ISREG(info.st_mode) or path.is_symlink() or info.st_nlink != 1 or
            info.st_uid != os.geteuid() or info.st_size > limit):
        raise ValueError(f"unsafe deployment input: {path}")
    data = path.read_bytes()
    after = path.lstat()
    if len(data) > limit or (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns) != (
            after.st_dev, after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns):
        raise ValueError(f"deployment input changed while reading: {path}")
    return data


def validate_environment(data: bytes):
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise ValueError("runtime environment is not UTF-8") from error
    seen = set()
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError("runtime environment contains a malformed assignment")
        key, value = line.split("=", 1)
        if key not in ENV_KEYS or key in seen or not value or not re.fullmatch(r"[A-Z0-9_]+", key):
            raise ValueError("runtime environment keys are missing, duplicate, unknown, or empty")
        seen.add(key)
    if seen != ENV_KEYS:
        raise ValueError(f"runtime environment fields mismatch: missing={sorted(ENV_KEYS-seen)}")


def validate_host_config(data: bytes):
    value = strict_json_bytes(data)
    if not isinstance(value, dict) or set(value) != HOST_FIELDS or value.get("schema_version") != 1:
        raise ValueError("host configuration fields or version mismatch")
    for name in ("state_directory", "socket_path", "kerf_executable", "multikernel_sysfs_root", "kernel_manifest_directory"):
        path = value[name]
        if not isinstance(path, str) or len(path) > 4096 or not path.startswith("/") or Path(os.path.normpath(path)) != Path(path):
            raise ValueError(f"host configuration path is unsafe: {name}")
    integers = ("min_primary_cpus", "min_primary_memory_bytes", "backend_timeout_seconds", "max_frame_size_bytes")
    if any(isinstance(value[name], bool) or not isinstance(value[name], int) for name in integers):
        raise ValueError("host configuration numeric field is not an integer")
    if (value["min_primary_cpus"] < 1 or value["min_primary_memory_bytes"] < 512 << 20 or
            not 1 <= value["backend_timeout_seconds"] <= 3600 or
            not 4096 <= value["max_frame_size_bytes"] <= 16 << 20):
        raise ValueError("host configuration numeric field is outside the v1 bounds")
    ids = value["forbidden_apic_ids"]
    if (not isinstance(ids, list) or not ids or 0 not in ids or
            any(isinstance(item, bool) or not isinstance(item, int) or item < 0 for item in ids) or
            len(ids) != len(set(ids))):
        raise ValueError("forbidden APIC IDs are invalid")


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def deployment_files(runtime_env: Path, host_config: Path):
    files = {name: secure_input(path) for name, path in ASSETS.items()}
    files["runtime.env"] = secure_input(runtime_env, 64 << 10)
    files["mkruntime/config.json"] = secure_input(host_config)
    validate_environment(files["runtime.env"])
    validate_host_config(files["mkruntime/config.json"])
    return files


def identifier(files) -> str:
    value = hashlib.sha256()
    for name in sorted(files):
        value.update(name.encode() + b"\0" + digest(files[name]).encode() + b"\n")
    return value.hexdigest()


def sync_directory(path: Path):
    descriptor = os.open(path, os.O_RDONLY | os.O_DIRECTORY)
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def atomic_symlink(target: str, path: Path):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.parent / f".{path.name}.multikernel-{os.getpid()}"
    if os.path.lexists(temporary):
        raise ValueError(f"temporary deployment link exists: {temporary}")
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


def link_target(destination: Path, internal: str) -> str:
    return os.path.relpath(BASE / "current" / internal, destination.parent)


def managed_link(root: Path, destination: Path, internal: str) -> bool:
    path = rooted(root, destination)
    return path.is_symlink() and os.readlink(path) == link_target(destination, internal)


def safe_directory(root: Path, absolute: Path, create: bool):
    current = root
    for component in absolute.relative_to("/").parts:
        current /= component
        if not os.path.lexists(current):
            if not create:
                return
            current.mkdir(mode=0o755)
        info = current.lstat()
        if (not stat.S_ISDIR(info.st_mode) or current.is_symlink() or
                info.st_uid != os.geteuid() or info.st_mode & 0o022):
            raise ValueError(f"unsafe deployment directory: {current}")


def preflight_links(root: Path, create_parents: bool = False):
    safe_directory(root, BASE, create_parents)
    current = rooted(root, BASE / "current")
    if os.path.lexists(current):
        if not current.is_symlink() or not re.fullmatch(r"deployments/[0-9a-f]{64}", os.readlink(current)):
            raise ValueError("deployment current selector is unmanaged")
        verify_deployment(root, os.readlink(current).split("/", 1)[1])
    for destination, internal in LINKS.items():
        safe_directory(root, destination.parent, create_parents)
        path = rooted(root, destination)
        if os.path.lexists(path) and not managed_link(root, destination, internal):
            raise ValueError(f"refuse unrelated deployment path: {path}")


def verify_deployment(root: Path, deployment_id: str):
    if not re.fullmatch(r"[0-9a-f]{64}", deployment_id):
        raise ValueError("invalid deployment identity")
    safe_directory(root, BASE / "deployments", False)
    directory = rooted(root, BASE / "deployments" / deployment_id)
    if not directory.is_dir() or directory.is_symlink():
        raise ValueError("installed deployment is unavailable")
    manifest = strict_json_bytes(secure_input(directory / "deployment-manifest.json"))
    if set(manifest) != {"schema_version", "deployment", "files"} or manifest["schema_version"] != 1 or manifest["deployment"] != deployment_id:
        raise ValueError("installed deployment manifest identity mismatch")
    if not isinstance(manifest["files"], dict) or set(manifest["files"]) != set(LINKS.values()):
        raise ValueError("installed deployment manifest file set mismatch")
    for name, expected in manifest["files"].items():
        data = secure_input(directory / name)
        if digest(data) != expected:
            raise ValueError(f"installed deployment file hash mismatch: {name}")
    return manifest


def activate(root: Path, deployment_id: str):
    preflight_links(root)
    verify_deployment(root, deployment_id)
    preflight_links(root, create_parents=True)
    created = []
    try:
        for destination, internal in LINKS.items():
            path = rooted(root, destination)
            if not managed_link(root, destination, internal):
                atomic_symlink(link_target(destination, internal), path)
                created.append(path)
        atomic_symlink(f"deployments/{deployment_id}", rooted(root, BASE / "current"))
    except BaseException:
        for path in reversed(created):
            try:
                path.unlink()
                sync_directory(path.parent)
            except FileNotFoundError:
                pass
        raise


def install(root: Path, runtime_env: Path, host_config: Path):
    preflight_links(root)
    files = deployment_files(runtime_env, host_config)
    deployment_id = identifier(files)
    preflight_links(root, create_parents=True)
    base = rooted(root, BASE)
    deployments = base / "deployments"
    safe_directory(root, BASE / "deployments", True)
    destination = deployments / deployment_id
    if destination.exists():
        verify_deployment(root, deployment_id)
    else:
        staging = deployments / f".{deployment_id}.staging-{os.getpid()}"
        staging.mkdir(mode=0o755)
        try:
            for name, data in files.items():
                output = staging / name
                output.parent.mkdir(parents=True, exist_ok=True, mode=0o755)
                mode = 0o600 if name in ("runtime.env", "mkruntime/config.json") else 0o755 if name in EXECUTABLES else 0o644
                descriptor = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, mode)
                with os.fdopen(descriptor, "wb") as stream:
                    stream.write(data)
                    stream.flush()
                    os.fsync(stream.fileno())
            manifest = {"schema_version": 1, "deployment": deployment_id, "files": {name: digest(data) for name, data in sorted(files.items())}}
            (staging / "deployment-manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
            os.chmod(staging / "deployment-manifest.json", 0o600)
            os.rename(staging, destination)
            sync_directory(deployments)
        except BaseException:
            shutil.rmtree(staging, ignore_errors=True)
            raise
        verify_deployment(root, deployment_id)
    activate(root, deployment_id)
    return deployment_id


def inspection(root: Path):
    preflight_links(root)
    base = rooted(root, BASE)
    current = base / "current"
    deployments = base / "deployments"
    safe_directory(root, BASE / "deployments", False)
    return {
        "active": os.readlink(current) if current.is_symlink() else None,
        "deployments": sorted(item.name for item in deployments.iterdir() if item.is_dir() and not item.name.startswith(".")) if deployments.is_dir() else [],
        "links": {str(path): managed_link(root, path, internal) for path, internal in LINKS.items()},
        "activation_required": ["systemctl daemon-reload", "systemctl enable --now sys-fs-multikernel.mount mkruntimed.service mknetd.service"],
    }


def uninstall(root: Path, apply: bool):
    report = inspection(root)
    report.update(operation="uninstall", applied=apply)
    if not apply:
        return report
    preflight_links(root)
    for destination, internal in LINKS.items():
        path = rooted(root, destination)
        if managed_link(root, destination, internal):
            path.unlink()
            sync_directory(path.parent)
    current = rooted(root, BASE / "current")
    if current.is_symlink():
        current.unlink()
        sync_directory(current.parent)
    report["preserved_deployments"] = inspection(root)["deployments"]
    return report


def remove_deployment(root: Path, deployment_id: str, apply: bool):
    verify_deployment(root, deployment_id)
    current = rooted(root, BASE / "current")
    if current.is_symlink() and os.readlink(current) == f"deployments/{deployment_id}":
        raise ValueError("refuse to remove the active deployment")
    if apply:
        directory = rooted(root, BASE / "deployments" / deployment_id)
        shutil.rmtree(directory)
        sync_directory(directory.parent)
    return {"operation": "remove-deployment", "deployment": deployment_id, "applied": apply}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, default=Path("/"), help=argparse.SUPPRESS)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("inspect")
    install_parser = commands.add_parser("install")
    install_parser.add_argument("runtime_env", type=Path)
    install_parser.add_argument("host_config", type=Path)
    activate_parser = commands.add_parser("activate")
    activate_parser.add_argument("deployment")
    uninstall_parser = commands.add_parser("uninstall")
    uninstall_parser.add_argument("--apply", action="store_true")
    remove_parser = commands.add_parser("remove-deployment")
    remove_parser.add_argument("deployment")
    remove_parser.add_argument("--apply", action="store_true")
    args = parser.parse_args()
    root = args.root.resolve(strict=True)
    mutation = args.command in ("install", "activate") or getattr(args, "apply", False)
    try:
        if mutation and root == Path("/") and os.geteuid() != 0:
            raise PermissionError("system mutation requires root")
        if args.command == "inspect":
            result = inspection(root)
        elif args.command == "install":
            result = {"installed_and_active": install(root, args.runtime_env, args.host_config)}
        elif args.command == "activate":
            activate(root, args.deployment)
            result = {"active": args.deployment}
        elif args.command == "uninstall":
            result = uninstall(root, args.apply)
        else:
            result = remove_deployment(root, args.deployment, args.apply)
        print(json.dumps(result, indent=2, sort_keys=True))
    except (OSError, ValueError, KeyError, TypeError) as error:
        print(f"manage runtime deployment: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
