#!/usr/bin/env python3
"""Exercise fresh install, upgrade, rollback, and safe removal transactions."""

import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


SCRIPT = Path(__file__).with_name("manage-runtime-binaries.py")
COMPONENTS = (
    "containerd-shim-multikernel-v2",
    "mk-agent",
    "mk-agentctl",
    "mk-cni",
    "mk-host-check",
    "mknetd",
    "mkruntimed",
)
LINKS = (
    "usr/local/bin/containerd-shim-multikernel-v2",
    "usr/local/bin/mk-agentctl",
    "usr/local/bin/mk-host-check",
    "usr/local/sbin/mkruntimed",
    "usr/local/sbin/mknetd",
    "opt/cni/bin/multikernel",
)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def make_release(work, version, revision):
    binary_dir = work / f"bin-{version}"
    binary_dir.mkdir()
    entries = []
    for name in COMPONENTS:
        path = binary_dir / name
        path.write_text(
            f"#!/bin/sh\nprintf '%s\\n' '{name} version={version} revision={revision}'\n",
            encoding="utf-8",
        )
        path.chmod(0o755)
        entries.append(
            {
                "name": name,
                "revision": revision,
                "sha256": digest(path),
                "size_bytes": path.stat().st_size,
                "version": version,
            }
        )
    manifest = work / f"manifest-{version}.json"
    manifest.write_text(
        json.dumps(
            {
                "components": entries,
                "revision": revision,
                "runtime": "io.containerd.multikernel.v2",
                "schema_version": 1,
                "version": version,
            },
            indent=2,
            sort_keys=True,
        )
        + "\n",
        encoding="utf-8",
    )
    return manifest, binary_dir, f"{version}-{revision}"


def run(root, *arguments, ok=True):
    completed = subprocess.run(
        ["python3", os.fspath(SCRIPT), "--root", os.fspath(root), *arguments],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if ok and completed.returncode != 0:
        raise AssertionError(completed.stderr)
    if not ok and completed.returncode == 0:
        raise AssertionError(f"unexpected success: {completed.stdout}")
    return completed


def active(root):
    return os.readlink(root / "usr/local/lib/multikernel/current")


def assert_links(root):
    for relative in LINKS:
        path = root / relative
        assert path.is_symlink(), path
        target = Path(os.readlink(path))
        resolved = (
            root / target.relative_to("/")
            if target.is_absolute()
            else (path.parent / target).resolve()
        )
        assert resolved.is_file(), resolved


def main():
    with tempfile.TemporaryDirectory(prefix="runtime-install-test-") as temporary:
        work = Path(temporary)
        root = work / "root"
        root.mkdir()
        first = make_release(work, "1.0.0", "1" * 40)
        second = make_release(work, "2.0.0", "2" * 40)

        result = json.loads(run(root, "inspect").stdout)
        assert result["active"] is None and result["releases"] == []

        run(root, "install", os.fspath(first[0]), os.fspath(first[1]))
        assert active(root) == f"releases/{first[2]}"
        assert_links(root)
        first_hash = digest(
            root / "usr/local/lib/multikernel" / active(root) / "bin/mkruntimed"
        )

        run(root, "install", os.fspath(second[0]), os.fspath(second[1]))
        assert active(root) == f"releases/{second[2]}"
        assert_links(root)
        assert first_hash != digest(
            root / "usr/local/lib/multikernel" / active(root) / "bin/mkruntimed"
        )

        # The same version/revision may never silently identify different bits.
        collision_manifest, collision_bin, _ = make_release(
            work, "1.0.0-collision", "1" * 40
        )
        for path in collision_bin.iterdir():
            payload = path.read_text(encoding="utf-8").replace(
                "version=1.0.0-collision", "version=1.0.0"
            )
            path.write_text(payload + "# distinct build\n", encoding="utf-8")
            path.chmod(0o755)
        payload = json.loads(collision_manifest.read_text(encoding="utf-8"))
        payload["version"] = "1.0.0"
        for entry in payload["components"]:
            path = collision_bin / entry["name"]
            entry["version"] = "1.0.0"
            entry["size_bytes"] = path.stat().st_size
            entry["sha256"] = digest(path)
        collision_manifest.write_text(
            json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
        refused = run(
            root,
            "install",
            os.fspath(collision_manifest),
            os.fspath(collision_bin),
            ok=False,
        )
        assert "different manifest content" in refused.stderr
        assert active(root) == f"releases/{second[2]}"

        run(root, "activate", first[2])
        assert active(root) == f"releases/{first[2]}"
        assert_links(root)

        rejected = run(root, "remove-release", first[2], "--apply", ok=False)
        assert "active release" in rejected.stderr
        preview = json.loads(run(root, "uninstall").stdout)
        assert preview["applied"] is False
        assert_links(root)

        run(root, "uninstall", "--apply")
        assert not os.path.lexists(root / "usr/local/lib/multikernel/current")
        assert all(not os.path.lexists(root / relative) for relative in LINKS)
        assert (root / "usr/local/lib/multikernel/releases" / first[2]).is_dir()
        run(root, "remove-release", second[2], "--apply")
        assert not (root / "usr/local/lib/multikernel/releases" / second[2]).exists()

        # Refuse to take over an unrelated command path without changing state.
        collision_root = work / "collision-root"
        collision = collision_root / LINKS[0]
        collision.parent.mkdir(parents=True)
        collision.write_text("operator-owned\n", encoding="utf-8")
        refused = run(
            collision_root,
            "install",
            os.fspath(first[0]),
            os.fspath(first[1]),
            ok=False,
        )
        assert "unrelated existing command path" in refused.stderr
        assert collision.read_text(encoding="utf-8") == "operator-owned\n"
        assert not (collision_root / "usr/local/lib/multikernel").exists()

        # Reject a manifest/input mismatch before creating any release state.
        tamper_root = work / "tamper-root"
        tamper_root.mkdir()
        (first[1] / "mknetd").write_text("#!/bin/sh\nexit 9\n", encoding="utf-8")
        (first[1] / "mknetd").chmod(0o755)
        refused = run(
            tamper_root,
            "install",
            os.fspath(first[0]),
            os.fspath(first[1]),
            ok=False,
        )
        assert "version command failed" in refused.stderr
        assert not (tamper_root / "usr/local/lib/multikernel").exists()

        # Never follow an operator-controlled releases-directory symlink.
        symlink_root = work / "symlink-root"
        outside = work / "outside"
        (symlink_root / "usr/local/lib/multikernel").mkdir(parents=True)
        outside.mkdir()
        os.symlink(outside, symlink_root / "usr/local/lib/multikernel/releases")
        refused = run(symlink_root, "inspect", ok=False)
        assert "non-symlink directory" in refused.stderr
        assert list(outside.iterdir()) == []

    print("runtime binary lifecycle tests: PASS")


if __name__ == "__main__":
    main()
