#!/usr/bin/env python3
"""Exercise deployment install, upgrade, rollback, and ownership boundaries."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


REPO = Path(__file__).resolve().parent.parent
SCRIPT = REPO / "scripts/manage-runtime-deployment.py"
ENVIRONMENT = REPO / "deploy/systemd/runtime.env.example"
HOST_CONFIG = REPO / "docs/runtime/contracts/fixtures/host-config.valid.json"
DESTINATIONS = (
    "etc/multikernel/runtime.env",
    "etc/mkruntime/config.json",
    "etc/systemd/system/sys-fs-multikernel.mount",
    "etc/systemd/system/mkruntimed.service",
    "etc/systemd/system/mknetd.service",
    "etc/cni/net.d/10-multikernel.conf",
    "etc/containerd/conf.d/20-multikernel-runtime.toml",
    "usr/local/libexec/multikernel/build-runtime-container-initramfs.sh",
    "usr/local/libexec/multikernel/validate-runtime-oci.py",
    "usr/local/libexec/multikernel/materialize-runtime-binds.py",
    "usr/local/libexec/multikernel/validate-runtime-bootstrap.py",
    "usr/local/libexec/multikernel/validate-runtime-root.py",
    "usr/local/libexec/multikernel/validate-runtime-image.py",
    "usr/local/libexec/multikernel/build-runtime-rootfs.py",
    "usr/local/libexec/multikernel/runtime_safe_publish.py",
    "usr/local/libexec/multikernel/runtime-storage-identity.py",
    "usr/local/libexec/multikernel/build-runtime-storage.py",
    "usr/local/libexec/multikernel/verify-runtime-rootfs.py",
    "usr/local/libexec/multikernel/guest/mk-agent-init",
    "usr/local/libexec/multikernel/guest/runtime-mediated-init",
)


def run(root, *arguments, ok=True):
    completed = subprocess.run(
        ["python3", os.fspath(SCRIPT), "--root", os.fspath(root), *arguments],
        text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False,
    )
    if ok and completed.returncode != 0:
        raise AssertionError(completed.stderr)
    if not ok and completed.returncode == 0:
        raise AssertionError(f"unexpected success: {completed.stdout}")
    return completed


def inputs(work, suffix, egress):
    environment = work / f"runtime-{suffix}.env"
    environment.write_text(
        ENVIRONMENT.read_text(encoding="utf-8").replace("MKNETWORK_EGRESS=ens4", f"MKNETWORK_EGRESS={egress}"),
        encoding="utf-8",
    )
    environment.chmod(0o600)
    host = work / f"host-{suffix}.json"
    shutil.copyfile(HOST_CONFIG, host)
    host.chmod(0o600)
    return environment, host


def active(root):
    return os.readlink(root / "etc/multikernel/current")


def assert_links(root):
    for relative in DESTINATIONS:
        path = root / relative
        assert path.is_symlink(), path
        assert path.resolve().is_file(), path


def main():
    with tempfile.TemporaryDirectory(prefix="runtime-deployment-test-") as temporary:
        work = Path(temporary)
        root = work / "root"
        root.mkdir()
        first_inputs = inputs(work, "one", "ens4")
        second_inputs = inputs(work, "two", "ens5")

        assert json.loads(run(root, "inspect").stdout)["deployments"] == []
        first = json.loads(run(root, "install", *map(os.fspath, first_inputs)).stdout)["installed_and_active"]
        assert active(root) == f"deployments/{first}"
        assert_links(root)
        assert (root / "etc/multikernel/deployments" / first / "runtime.env").stat().st_mode & 0o777 == 0o600
        assert (root / "usr/local/libexec/multikernel/build-runtime-container-initramfs.sh").stat().st_mode & 0o777 == 0o755

        second = json.loads(run(root, "install", *map(os.fspath, second_inputs)).stdout)["installed_and_active"]
        assert second != first and active(root) == f"deployments/{second}"
        assert_links(root)
        run(root, "activate", first)
        assert active(root) == f"deployments/{first}"
        assert "MKNETWORK_EGRESS=ens4" in (root / "etc/multikernel/runtime.env").read_text()

        refused = run(root, "remove-deployment", first, "--apply", ok=False)
        assert "active deployment" in refused.stderr
        preview = json.loads(run(root, "uninstall").stdout)
        assert preview["applied"] is False
        run(root, "uninstall", "--apply")
        assert all(not os.path.lexists(root / relative) for relative in DESTINATIONS)
        assert (root / "etc/multikernel/deployments" / first).is_dir()
        run(root, "remove-deployment", second, "--apply")
        assert not (root / "etc/multikernel/deployments" / second).exists()
        tampered = root / "etc/multikernel/deployments" / first / "systemd/mknetd.service"
        tampered.write_text("tampered\n")
        refused = run(root, "activate", first, ok=False)
        assert "hash mismatch" in refused.stderr

        collision_root = work / "collision-root"
        collision = collision_root / DESTINATIONS[2]
        collision_root.mkdir()
        (collision_root / "etc").mkdir(mode=0o755)
        (collision_root / "etc/systemd").mkdir(mode=0o755)
        collision.parent.mkdir(mode=0o755)
        collision.write_text("operator-owned\n")
        refused = run(collision_root, "install", *map(os.fspath, first_inputs), ok=False)
        assert "unrelated deployment path" in refused.stderr, refused.stderr
        assert collision.read_text() == "operator-owned\n"
        assert not (collision_root / "etc/multikernel").exists()

        invalid_root = work / "invalid-root"
        invalid_root.mkdir()
        invalid_env = work / "invalid.env"
        invalid_env.write_text("UNKNOWN=value\n")
        invalid_env.chmod(0o600)
        refused = run(invalid_root, "install", os.fspath(invalid_env), os.fspath(first_inputs[1]), ok=False)
        assert "environment" in refused.stderr
        assert not (invalid_root / "etc/multikernel").exists()

        symlink_root = work / "symlink-root"
        outside = work / "outside"
        outside.mkdir()
        (symlink_root / "etc").mkdir(parents=True)
        os.symlink(outside, symlink_root / "etc/multikernel")
        refused = run(symlink_root, "install", *map(os.fspath, first_inputs), ok=False)
        assert "unsafe deployment directory" in refused.stderr
        assert list(outside.iterdir()) == []

        selector_root = work / "selector-root"
        current = selector_root / "etc/multikernel/current"
        selector_root.mkdir()
        (selector_root / "etc").mkdir(mode=0o755)
        current.parent.mkdir(mode=0o755)
        os.symlink("deployments/" + "a" * 64, current)
        refused = run(selector_root, "install", *map(os.fspath, first_inputs), ok=False)
        assert "installed deployment is unavailable" in refused.stderr

    print("runtime deployment lifecycle tests: PASS")


if __name__ == "__main__":
    main()
