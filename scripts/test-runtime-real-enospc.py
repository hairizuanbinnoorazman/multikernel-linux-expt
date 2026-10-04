#!/usr/bin/env python3
"""Exercise builders against a genuinely constrained filesystem.

The helper overrides only the builders' admission-capacity observation so the
kernel, rather than a mock, returns ENOSPC at the requested allocation boundary.
It is intended for the privileged disposable-host qualification driver.
"""

from __future__ import annotations

import argparse
import errno
import importlib.util
import os
from pathlib import Path
import sys
from types import SimpleNamespace
from unittest import mock


SCRIPT_DIR = Path(__file__).resolve().parent


def load(name: str, filename: str):
    spec = importlib.util.spec_from_file_location(name, SCRIPT_DIR / filename)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def assert_clean(directory: Path, *public: Path) -> None:
    for path in public:
        if path.exists():
            raise AssertionError(f"failed attempt left public output: {path}")
    residue = [
        path.name for path in directory.iterdir()
        if path.name.startswith((".root-staging.", ".root.ext4.", ".root.json.", ".initramfs.", ".manifest."))
    ]
    if residue:
        raise AssertionError(f"failed attempt left private residue: {sorted(residue)}")


def require_enospc(error: BaseException) -> None:
    values: list[BaseException] = [error]
    while values:
        current = values.pop()
        if isinstance(current, OSError) and current.errno == errno.ENOSPC:
            return
        values.extend(item for item in (current.__cause__, current.__context__) if item is not None)
    if "No space left on device" not in str(error):
        raise AssertionError(f"failure was not ENOSPC: {error!r}") from error


def storage(root: Path, output_dir: Path) -> None:
    builder = load("runtime_storage_real_enospc", "build-runtime-storage.py")
    output = output_dir / "root.ext4"
    metadata = output_dir / "root.json"
    arguments = SimpleNamespace(
        root=root,
        output=output,
        metadata=metadata,
        logical_path=Path("/var/lib/multikernel/rootfs/live-enospc/root.ext4"),
        image_id="live-enospc",
        uuid="11111111-2222-4333-8444-555555555555",
        size=64 << 20,
        inodes=4096,
        port=4061,
        min_free_bytes=0,
        min_free_inodes=0,
        mke2fs="/usr/sbin/mke2fs",
        e2fsck="/usr/sbin/e2fsck",
        debugfs="/usr/sbin/debugfs",
    )
    try:
        with mock.patch.object(builder, "filesystem_capacity", return_value=(1 << 40, 1 << 30)):
            builder.build(arguments)
    except BaseException as error:
        require_enospc(error)
    else:
        raise AssertionError("constrained storage build unexpectedly succeeded")
    assert_clean(output_dir, output, metadata)
    print("REAL_STORAGE_ENOSPC_CLEAN")


def rootfs(root: Path, output_dir: Path) -> None:
    builder = load("runtime_rootfs_real_enospc", "build-runtime-rootfs.py")
    output = output_dir / "initramfs.cpio.gz"
    manifest = output_dir / "manifest.json"
    arguments = [str(SCRIPT_DIR / "build-runtime-rootfs.py"), str(root), str(output), str(manifest)]
    capacity = os.statvfs(output_dir)
    reported = SimpleNamespace(
        f_bavail=1 << 30,
        f_frsize=max(capacity.f_frsize, 4096),
        f_favail=1 << 30,
    )
    try:
        with mock.patch.object(builder.os, "statvfs", return_value=reported), mock.patch.object(sys, "argv", arguments):
            builder.main()
    except BaseException as error:
        require_enospc(error)
    else:
        raise AssertionError("constrained initramfs build unexpectedly succeeded")
    assert_clean(output_dir, output, manifest)
    print("REAL_INITRAMFS_ENOSPC_CLEAN")


def fill_inodes(directory: Path) -> int:
    created: list[Path] = []
    for index in range(1 << 20):
        path = directory / f"filler-{index:08d}"
        try:
            descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_CLOEXEC, 0o600)
        except OSError as error:
            if error.errno != errno.ENOSPC:
                raise
            break
        else:
            os.close(descriptor)
            created.append(path)
    else:
        raise AssertionError("inode-constrained filesystem did not reach ENOSPC")
    if not created:
        raise AssertionError("inode-constrained filesystem had no removable filler inode")
    created[-1].unlink()
    return len(created) - 1


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("storage", "rootfs", "rootfs-inodes"))
    parser.add_argument("root", type=Path)
    parser.add_argument("output_dir", type=Path)
    arguments = parser.parse_args()
    if arguments.mode == "storage":
        storage(arguments.root, arguments.output_dir)
    elif arguments.mode == "rootfs":
        rootfs(arguments.root, arguments.output_dir)
    else:
        count = fill_inodes(arguments.output_dir)
        print(f"INODE_FILLERS_RETAINED={count}")
        rootfs(arguments.root, arguments.output_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
