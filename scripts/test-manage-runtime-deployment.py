#!/usr/bin/env python3
"""Exercise deployment install, upgrade, rollback, and ownership boundaries."""

import json
import hashlib
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
    "etc/systemd/system/multikernel-modules.service",
    "etc/systemd/system/mknetd.service",
    "etc/cni/net.d/10-multikernel.conf",
    "etc/containerd/conf.d/20-multikernel-runtime.toml",
    "usr/local/libexec/multikernel/build-runtime-container-initramfs.sh",
    "usr/local/libexec/multikernel/validate-runtime-oci.py",
    "usr/local/libexec/multikernel/materialize-runtime-binds.py",
    "usr/local/libexec/multikernel/validate-runtime-bootstrap.py",
    "usr/local/libexec/multikernel/validate-runtime-docker-root.py",
    "usr/local/libexec/multikernel/validate-runtime-root.py",
    "usr/local/libexec/multikernel/validate-runtime-image.py",
    "usr/local/libexec/multikernel/build-runtime-rootfs.py",
    "usr/local/libexec/multikernel/runtime_safe_publish.py",
    "usr/local/libexec/multikernel/runtime_support_exec.py",
    "usr/local/libexec/multikernel/runtime-storage-identity.py",
    "usr/local/libexec/multikernel/validate-runtime-storage-mount.py",
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
        runtime_unit = (root / "etc/systemd/system/mkruntimed.service").resolve().read_text(encoding="utf-8")
        assert runtime_unit.count("\nKillMode=process\n") == 1
        assert runtime_unit.count("\nRequires=google-guest-agent.service\n") == 1
        after = next(line for line in runtime_unit.splitlines() if line.startswith("After="))
        assert after.split().count("google-guest-agent.service") == 1

        # Updating the approved disk size clones the verified installed set;
        # no current repository assets are read, even from a standalone tool.
        original_manifest = json.loads((root / "etc/multikernel/deployments" / first / "deployment-manifest.json").read_text())
        original_env = (root / "etc/multikernel/runtime.env").read_bytes()
        old_size = int(next(line.split(b"=", 1)[1] for line in original_env.splitlines() if line.startswith(b"MKRUNTIME_STORAGE_BYTES=")))
        larger = old_size + (10 << 30)
        standalone = work / "standalone" / "manager.py"; standalone.parent.mkdir(); shutil.copyfile(SCRIPT, standalone)
        grown_run = subprocess.run(["python3", str(standalone), "--root", str(root), "update-storage-size", str(old_size), str(larger)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        assert grown_run.returncode == 0, grown_run.stderr
        grown = json.loads(grown_run.stdout); grown_id = grown["active"]
        assert grown["previous"] == first and grown["changed_files"] == ["runtime.env"]
        assert not grown["disk_resize_performed"] and grown["service_restart_required"]
        assert active(root) == "deployments/" + grown_id
        grown_manifest = json.loads((root / "etc/multikernel/deployments" / grown_id / "deployment-manifest.json").read_text())
        assert [name for name in original_manifest["files"] if original_manifest["files"][name] != grown_manifest["files"][name]] == ["runtime.env"]
        assert (root / "etc/multikernel/runtime.env").read_bytes() == original_env.replace(str(old_size).encode(), str(larger).encode())
        assert (root / "etc/multikernel/deployments" / first / "runtime.env").read_bytes() == original_env
        for name in original_manifest["files"]:
            assert (root / "etc/multikernel/deployments" / first / name).stat().st_mode & 0o777 == (root / "etc/multikernel/deployments" / grown_id / name).stat().st_mode & 0o777
        for expected, target in [(old_size, larger), (larger, old_size), (larger, larger+1)]:
            run(root, "update-storage-size", str(expected), str(target), ok=False)
            assert active(root) == "deployments/" + grown_id
        run(root, "activate", first)
        print("IMMUTABLE_STORAGE_SIZE_ONLY_CLONE_STANDALONE_AND_STALE_SHRINK_ALIGNMENT_REFUSALS_PASS")

        second = json.loads(run(root, "install", *map(os.fspath, second_inputs)).stdout)["installed_and_active"]
        assert second != first and active(root) == f"deployments/{second}"
        assert_links(root)
        run(root, "activate", first)
        assert active(root) == f"deployments/{first}"
        assert "MKNETWORK_EGRESS=ens4" in (root / "etc/multikernel/runtime.env").read_text()

        full = root / "etc/multikernel/deployments" / first
        legacy_staging = root / "etc/multikernel/deployments/legacy-staging"
        shutil.copytree(full, legacy_staging)
        legacy_manifest_path = legacy_staging / "deployment-manifest.json"
        legacy_manifest = json.loads(legacy_manifest_path.read_text(encoding="utf-8"))
        removed_assets = ("libexec/validate-runtime-storage-mount.py", "libexec/runtime_support_exec.py", "systemd/multikernel-modules.service")
        for removed_asset in removed_assets:
            legacy_manifest["files"].pop(removed_asset)
        identity = hashlib.sha256()
        for name in sorted(legacy_manifest["files"]):
            expected = legacy_manifest["files"][name]
            identity.update(name.encode() + b"\0" + expected.encode() + b"\n")
        legacy = identity.hexdigest()
        legacy_manifest["deployment"] = legacy
        legacy_manifest_path.write_text(
            json.dumps(legacy_manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        for removed_asset in removed_assets:
            (legacy_staging / removed_asset).unlink()
        legacy_directory = legacy_staging.with_name(legacy)
        legacy_staging.rename(legacy_directory)
        run(root, "activate", legacy)
        assert active(root) == f"deployments/{legacy}"
        assert not os.path.lexists(root / "usr/local/libexec/multikernel/validate-runtime-storage-mount.py")
        assert not os.path.lexists(root / "usr/local/libexec/multikernel/runtime_support_exec.py")
        assert not os.path.lexists(root / "etc/systemd/system/multikernel-modules.service")
        run(root, "activate", second)
        assert active(root) == f"deployments/{second}"
        assert (root / "usr/local/libexec/multikernel/validate-runtime-storage-mount.py").resolve().is_file()
        assert (root / "usr/local/libexec/multikernel/runtime_support_exec.py").resolve().is_file()
        assert (root / "etc/systemd/system/multikernel-modules.service").resolve().is_file()
        run(root, "activate", first)

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

        for name, old, new in (
            ("storage-mount", "MKRUNTIME_STORAGE_MOUNT=/srv/multikernel-storage", "MKRUNTIME_STORAGE_MOUNT=/tmp/storage"),
            ("storage-bytes", "MKRUNTIME_STORAGE_BYTES=21474836480", "MKRUNTIME_STORAGE_BYTES=513"),
            ("storage-uuid", "MKRUNTIME_STORAGE_UUID=507c0523-8e58-4ae3-9524-3b7513aad344", "MKRUNTIME_STORAGE_UUID=invalid"),
        ):
            invalid_root = work / f"invalid-{name}-root"
            invalid_root.mkdir()
            invalid_env = work / f"invalid-{name}.env"
            invalid_env.write_text(
                ENVIRONMENT.read_text(encoding="utf-8").replace(old, new), encoding="utf-8")
            invalid_env.chmod(0o600)
            refused = run(
                invalid_root, "install", os.fspath(invalid_env),
                os.fspath(first_inputs[1]), ok=False)
            assert "storage" in refused.stderr

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
