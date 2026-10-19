#!/usr/bin/env python3
"""Reject malformed native helper arguments before network/device operations."""

from pathlib import Path
import hashlib
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parent.parent


def main():
    with tempfile.TemporaryDirectory(prefix="mk-native-args-") as temporary:
        build = Path(temporary)
        for name in ("mkvsock-nbd", "mkvsock-relay", "mkvsock-probe", "argument-boundaries"):
            source = (ROOT / "scripts/test-native-helper-arguments.c" if name == "argument-boundaries"
                      else ROOT / "tools" / (name + ".c"))
            subprocess.run(["cc", "-O2", "-Wall", "-Wextra", "-Werror", str(source), "-o", str(build / name)], check=True)
        subprocess.run([str(build / "argument-boundaries")], check=True, timeout=5)
        canary = build / "host-canary"
        canary.write_bytes(b"unchanged-host-canary" * 256)
        original = hashlib.sha256(canary.read_bytes()).digest()
        socket_path = build / "relay.sock"
        cases = []
        bad_numbers = ("", "-1", "+1", " 1", "1 ", "1x", "0x10", "4294967295",
                       "4294967296", "18446744073709551616", "9" * 200)
        for value in bad_numbers:
            cases.extend([
                ("mkvsock-nbd", ["server", str(canary), value, "image", "generation", "1"]),
                ("mkvsock-nbd", ["client", value, "4061", str(canary), "4096", "image", "generation", "reserved"]),
                ("mkvsock-relay", ["server", value, str(socket_path)]),
                ("mkvsock-relay", ["client", value, "4061", str(socket_path)]),
                ("mkvsock-probe", ["server", value]),
                ("mkvsock-probe", ["client", value, "4061"]),
            ])
        for value in ("0", *bad_numbers):
            cases.append(("mkvsock-nbd", ["server", str(canary), "4061", "image", "generation", value]))
        cases.append(("mkvsock-nbd", ["server", str(canary), "4061", "image", "generation"]))
        for size in ("0", "4095", "4097", "-4096", "4096x", "18446744073709551616"):
            cases.append(("mkvsock-nbd", ["client", "0", "4061", str(canary), size, "image", "generation", "reserved"]))
        for identity in ("", "-identity", "id\n", "id/path", "x" * 64):
            cases.append(("mkvsock-nbd", ["server", str(canary), "4061", identity, "generation", "1"]))
        cases.extend([
            ("mkvsock-relay", ["server", "4061", ""]),
            ("mkvsock-relay", ["client", "0", "4061", "x" * 108]),
            ("mkvsock-nbd", ["client", "0", "4061", str(canary), "4096", "image", "generation", "unexpected"]),
        ])
        for index, (name, arguments) in enumerate(cases):
            result = subprocess.run([str(build / name), *arguments], capture_output=True, timeout=5)
            if result.returncode != 2 or not result.stderr.startswith(b"invalid "):
                raise AssertionError(f"native argument case {index} did not reject before host operations: {result.returncode}")
        assert hashlib.sha256(canary.read_bytes()).digest() == original
        assert not socket_path.exists()
        print(f"NATIVE_HELPER_CLI_REJECTION_PASS cases={len(cases)} host_canary=unchanged socket=absent")


if __name__ == "__main__":
    main()
