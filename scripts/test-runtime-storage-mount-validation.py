#!/usr/bin/env python3
"""Focused parser and deployment tests for the storage-mount precondition."""

from __future__ import annotations

import importlib.util
from pathlib import Path
import subprocess
import tempfile


REPO = Path(__file__).resolve().parent.parent
VALIDATOR = REPO / "scripts/validate-runtime-storage-mount.py"
spec = importlib.util.spec_from_file_location("storage_mount_validator", VALIDATOR)
module = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(module)


def main():
    sample = (
        "41 23 8:16 / /srv/multikernel-storage rw,nosuid - ext4 /dev/sdb rw\n"
        "42 23 0:44 / /path\\040with\\040spaces ro - tmpfs tmpfs ro\n"
    )
    entries = module.mount_entries(sample)
    assert entries[0] == {
        "device": "8:16", "mountpoint": "/srv/multikernel-storage",
        "options": {"rw", "nosuid"}, "type": "ext4", "source": "/dev/sdb",
    }
    assert entries[1]["mountpoint"] == "/path with spaces"
    assert module.udev_serial("E:OTHER=value\nE:ID_SERIAL_SHORT=disk-one\n") == "disk-one"
    for serial in ("", "E:ID_SERIAL_SHORT=bad value\n", "E:ID_SERIAL_SHORT=one\nE:ID_SERIAL_SHORT=two\n"):
        try:
            module.udev_serial(serial)
        except ValueError:
            pass
        else:
            raise AssertionError("unsafe udev serial was accepted")
    for malformed in ("", "invalid", "1 2 3 4 5 6 - ext4"):
        try:
            module.mount_entries(malformed)
        except ValueError:
            pass
        else:
            if malformed:
                raise AssertionError("malformed mountinfo was accepted")
    with tempfile.TemporaryDirectory(prefix="mk-storage-mount-") as temporary:
        missing = Path(temporary) / "missing"
        result = subprocess.run([
            str(VALIDATOR), "--mountpoint", "/srv/multikernel-storage",
            "--device", str(missing), "--serial", "disk-one",
            "--bytes", "21474836480", "--label", "mk-mediated-host",
            "--uuid", "507c0523-8e58-4ae3-9524-3b7513aad344",
        ], text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        assert result.returncode == 1
        assert "runtime storage mount rejected" in result.stderr
    service = (REPO / "deploy/systemd/mkruntimed.service").read_text(encoding="utf-8")
    assert "ExecStartPre=/usr/local/libexec/multikernel/validate-runtime-storage-mount.py" in service
    print("runtime storage mount validation: PASS (mountinfo, fail-closed input, service wiring)")


if __name__ == "__main__":
    main()
