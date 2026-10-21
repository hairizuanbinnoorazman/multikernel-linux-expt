#!/usr/bin/env python3
"""Focused tests for approved agent/relay/transport bootstrap resolution."""

import copy
import hashlib
import gzip
import io
import json
import os
import pathlib
import shutil
import struct
import runpy
import subprocess
import sys
import tempfile


REPO = pathlib.Path(__file__).resolve().parent.parent
VALIDATOR = pathlib.Path(os.environ.get("MK_TEST_BOOTSTRAP_VALIDATOR", REPO / "scripts/validate-runtime-bootstrap.py"))


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def kernel_bytes(banner=b"Linux version test-release (fixture)\n\x00"):
    names = b"\x00.shstrtab\x00.strtab\x00.symtab\x00.rodata\x00"
    strings = b"\x00linux_banner\x00"
    strings_offset = 64 + len(names)
    symbols_offset = (strings_offset + len(strings) + 7) & ~7
    banner_offset = symbols_offset + 48
    sections_offset = (banner_offset + len(banner) + 7) & ~7
    data = bytearray(sections_offset + 5 * 64)
    data[:7] = b"\x7fELF\x02\x01\x01"
    struct.pack_into("<HHI", data, 16, 2, 62, 1)
    struct.pack_into("<Q", data, 40, sections_offset)
    struct.pack_into("<H", data, 52, 64)
    struct.pack_into("<HHH", data, 58, 64, 5, 1)
    data[64:64 + len(names)] = names
    data[strings_offset:strings_offset + len(strings)] = strings
    data[banner_offset:banner_offset + len(banner)] = banner
    address = 0xffffffff81000000
    struct.pack_into("<IBBHQQ", data, symbols_offset + 24, 1, 0x11, 0, 4, address, len(banner))
    for i, (name, kind, offset, size) in enumerate([
        (1, 3, 64, len(names)), (11, 3, strings_offset, len(strings)),
        (19, 2, symbols_offset, 48), (27, 1, banner_offset, len(banner)),
    ], 1):
        struct.pack_into("<IIQQQQIIQQ", data, sections_offset + i * 64,
                         name, kind, 2 if i == 4 else 0, address if i == 4 else 0,
                         offset, size, 2 if i == 3 else 0, 0, 0, 24 if i == 3 else 0)
    return data


def unused_symbol(oversized):
    data = kernel_bytes()
    sections = struct.unpack_from("<Q", data, 40)[0]
    table = sections + 3 * 64
    symbols_offset = struct.unpack_from("<Q", data, table + 24)[0]
    symbols = data[symbols_offset:symbols_offset + 48]
    name = 0xffffffff
    if oversized:
        strings_offset, strings_size = struct.unpack_from("<QQ", data, sections + 2 * 64 + 24)
        text = data[strings_offset:strings_offset + strings_size] + b"x" * 4096 + b"\x00"
        struct.pack_into("<QQ", data, sections + 2 * 64 + 24, len(data), len(text))
        data.extend(text)
        name = strings_size
    struct.pack_into("<QQ", data, table + 24, len(data), 72)
    data.extend(symbols + struct.pack("<IBBHQQ", name, 0, 0, 0, 0, 0))
    return data



def archive_bytes(entries):
    data = bytearray()
    for name, content, mode, uid, links in [*entries, ("TRAILER!!!", b"", 0, 0, 0)]:
        encoded = name.encode() + b"\x00"
        fields = (1, mode, uid, 0, links, 0, len(content), 0, 0, 0, 0, len(encoded), 0)
        data.extend(b"070701" + b"".join(f"{value:08x}".encode() for value in fields))
        data.extend(encoded)
        data.extend(bytes((-len(data)) % 4))
        data.extend(content)
        data.extend(bytes((-len(data)) % 4))
    return gzip.compress(data, mtime=0)

def validate(path):
    return subprocess.run(
        [sys.executable, str(VALIDATOR), str(path), "test", "--required-uid", str(os.getuid()), "--skip-modinfo", "--skip-parent-safety"],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def main():
    with tempfile.TemporaryDirectory(prefix="mk-bootstrap-") as temporary:
        directory = pathlib.Path(temporary)
        executable = pathlib.Path(sys.executable).resolve()
        artifacts = {}
        for name in ("vmlinux", "mk-agent", "mkvsock-relay"):
            target = directory / name
            if name == "vmlinux":
                target.write_bytes(kernel_bytes())
            else:
                shutil.copyfile(executable, target)
            target.chmod(0o755)
            artifacts[name] = {"path": str(target), "sha256": digest(target)}
        initramfs = directory / "initramfs"
        module = directory / "mk_transport.ko"
        initramfs.write_bytes(archive_bytes([
            (name, (directory / name).read_bytes(), 0o100755, 0, 1)
            for name in ("mk-agent", "mkvsock-relay")
        ] + [("mk_transport.ko", b"module", 0o100644, 0, 1)]))
        module.write_bytes(b"module")
        initramfs.chmod(0o644)
        module.chmod(0o644)
        manifest = {
            "schema_version": 1,
            "name": "test",
            "architecture": "amd64",
            "kernel_release": "test-release",
            "compatibility": {"multikernel_revision": "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7", "kerf_version": "v0.2.0", "kerf_revision": "8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec"},
            "kernel": artifacts["vmlinux"],
            "initramfs": {"path": str(initramfs), "sha256": digest(initramfs)},
            "agent": artifacts["mk-agent"],
            "relay": artifacts["mkvsock-relay"],
            "transport": {"module": {"path": str(module), "sha256": digest(module)}, "module_name": "mk_transport", "socket_option": 9, "transport_id": 1, "primary_role": "server", "child_role": "client"},
            "protocol": {"min": 1, "max": 1},
            "required_config": ["CONFIG_MULTIKERNEL=y"],
            "oci_features": ["argv"],
        }
        path = directory / "test.json"
        config = directory / "config-test-release"
        config.write_text("CONFIG_MULTIKERNEL=y\n", encoding="utf-8")
        config.chmod(0o644)
        path.write_text(json.dumps(manifest), encoding="utf-8")
        path.chmod(0o600)
        result = validate(path)
        if result.returncode != 0:
            raise AssertionError(result.stderr)
        resolved = json.loads(result.stdout)
        if resolved["relay"] != str(directory / "mkvsock-relay") or resolved["module"] != str(module):
            raise AssertionError(resolved)

        bootstrap = directory / "docker-bootstrap"
        bootstrap.write_bytes(b"#!/bin/busybox sh\nexit 0\n")
        bootstrap.chmod(0o755)
        docker = {
            "kind": "docker-rootless-offline-v1",
            "rootfs_manifest_sha256": "a" * 64,
            "bootstrap": {"path": str(bootstrap), "sha256": digest(bootstrap)},
            "modules": {name: {"path": str(module), "sha256": digest(module)}
                        for name in ("llc", "stp", "bridge", "veth")},
        }
        # These structural fixtures explicitly skip modinfo, as do the existing
        # transport fixtures. Go ELF fixtures and live tests cover module names.
        candidate = copy.deepcopy(manifest)
        candidate["docker_guest"] = docker
        path.write_text(json.dumps(candidate))
        result = validate(path)
        if result.returncode != 0 or json.loads(result.stdout)["docker_guest"] != docker:
            raise AssertionError("approved Docker metadata was not preserved: " + result.stderr)
        for name, change in (
            ("null", lambda value: value.update(docker_guest=None)),
            ("root alias shadow", lambda value: value.update(DOCKER_GUEST=None)),
            ("kind alias", lambda value: value["docker_guest"].update(KIND="docker-rootless-offline-v1")),
            ("bootstrap alias", lambda value: value["docker_guest"]["bootstrap"].update(PATH=str(bootstrap))),
            ("kind", lambda value: value["docker_guest"].update(kind="docker-rootful-v1")),
            ("digest", lambda value: value["docker_guest"].update(rootfs_manifest_sha256="latest")),
            ("uppercase", lambda value: value["docker_guest"].update(rootfs_manifest_sha256="A" * 64)),
            ("unknown", lambda value: value["docker_guest"].update(hook="arbitrary-command")),
            ("missing module", lambda value: value["docker_guest"]["modules"].pop("bridge")),
            ("extra module", lambda value: value["docker_guest"]["modules"].update(other=docker["modules"]["llc"])),
            ("bootstrap hash", lambda value: value["docker_guest"]["bootstrap"].update(sha256="0" * 64)),
        ):
            candidate = copy.deepcopy(manifest)
            candidate["docker_guest"] = copy.deepcopy(docker)
            change(candidate)
            path.write_text(json.dumps(candidate))
            if validate(path).returncode == 0:
                raise AssertionError("unsafe Docker guest approval accepted: " + name)
        bootstrap.write_bytes(b"#!/bin/sh\nexit 0\n")
        candidate = copy.deepcopy(manifest)
        candidate["docker_guest"] = copy.deepcopy(docker)
        candidate["docker_guest"]["bootstrap"]["sha256"] = digest(bootstrap)
        path.write_text(json.dumps(candidate))
        if validate(path).returncode == 0:
            raise AssertionError("unapproved Docker bootstrap interpreter accepted")

        for name, mutate in (
            ("direction", lambda value: value["transport"].update(child_role="server")),
            ("digest", lambda value: value["relay"].update(sha256="0" * 64)),
            ("null digest", lambda value: value["relay"].update(sha256=None)),
            ("unknown", lambda value: value.update(unknown=True)),
            ("required config", lambda value: value.update(required_config=["CONFIG_MKTTY=y"])),
            ("unsafe release", lambda value: value.update(kernel_release="../escape")),
        ):
            candidate = copy.deepcopy(manifest)
            mutate(candidate)
            path.write_text(json.dumps(candidate), encoding="utf-8")
            if validate(path).returncode == 0:
                raise AssertionError(f"{name} mismatch accepted")

        kernel = directory / "vmlinux"
        for name, data in [
            ("wrong release", kernel_bytes(b"Linux version wrong-release (fixture)\n\x00")),
            ("prefix alias", kernel_bytes(b"Linux version test-release-extra (fixture)\n\x00")),
            ("unterminated", kernel_bytes(b"Linux version test-release (fixture)\n")),
            ("embedded NUL", kernel_bytes(b"Linux version test-release \x00forged\x00")),
            ("unused symbol bounds", unused_symbol(False)),
            ("unused symbol length", unused_symbol(True)),
        ]:
            kernel.write_bytes(data)
            candidate = copy.deepcopy(manifest)
            candidate["kernel"]["sha256"] = digest(kernel)
            path.write_text(json.dumps(candidate))
            result = validate(path)
            if result.returncode == 0 or "bootstrap validation failed:" not in result.stderr:
                raise AssertionError(f"{name}: {result.stderr!r}")
        for name, mutate in [
            ("relocatable", lambda data, sections, symbols: struct.pack_into("<H", data, 16, 1)),
            ("wrong machine", lambda data, sections, symbols: struct.pack_into("<H", data, 18, 183)),
            ("oversized symbol table", lambda data, sections, symbols: struct.pack_into("<Q", data, sections + 3 * 64 + 32, 33 << 20)),
            ("invalid table link", lambda data, sections, symbols: struct.pack_into("<I", data, sections + 3 * 64 + 40, 99)),
            ("compressed names", lambda data, sections, symbols: struct.pack_into("<Q", data, sections + 2 * 64 + 8, 0x800)),
            ("invalid symbol name", lambda data, sections, symbols: struct.pack_into("<I", data, symbols + 24, 0xffffffff)),
            ("invalid reserved symbol", lambda data, sections, symbols: data.__setitem__(symbols, 1)),
            ("nobits release", lambda data, sections, symbols: struct.pack_into("<I", data, sections + 4 * 64 + 4, 8)),
            ("outside section", lambda data, sections, symbols: struct.pack_into("<Q", data, symbols + 24 + 8, 0xffffffff82000000)),
        ]:
            data = kernel_bytes()
            sections = struct.unpack_from("<Q", data, 40)[0]
            symbols = struct.unpack_from("<Q", data, sections + 3 * 64 + 24)[0]
            mutate(data, sections, symbols)
            kernel.write_bytes(data)
            candidate = copy.deepcopy(manifest)
            candidate["kernel"]["sha256"] = digest(kernel)
            path.write_text(json.dumps(candidate))
            if validate(path).returncode == 0:
                raise AssertionError(f"{name}: malformed hash-approved kernel accepted")
        kernel.write_bytes(kernel_bytes())
        manifest["kernel"]["sha256"] = digest(kernel)

        # Verify actual descriptor mutation detection independently of pathname
        # preflight. Read through the held file, then mutate that same inode.
        api = runpy.run_path(str(VALIDATOR))
        held = directory / "held-artifact"
        held.write_bytes(b"approved")
        held.chmod(0o600)
        try:
            with api["verified_file"](str(held), digest(held), os.getuid(), True) as stream:
                if stream.read() != b"approved":
                    raise AssertionError("held descriptor changed bytes")
                held.write_bytes(b"mutated!")
        except ValueError as error:
            if "changed during verified read" not in str(error):
                raise
        else:
            raise AssertionError("mutation of held inode was accepted")

        # Each malformed archive is independently hash-approved. A whole-file
        # digest must not disguise a mismatched critical component or unsafe
        # extraction semantics. No archive is extracted by the verifier.
        entries = [(name, (directory / name).read_bytes(), 0o100755, 0, 1)
                   for name in ("mk-agent", "mkvsock-relay", "mk_transport.ko")]
        good_archive = archive_bytes(entries)
        aliased_trailer = bytearray(gzip.decompress(good_archive))
        trailer_name = aliased_trailer.rindex(b"TRAILER!!!")
        aliased_trailer[trailer_name - 16:trailer_name - 8] = b"0000000d"
        aliased_trailer[trailer_name:trailer_name + 13] = b"./TRAILER!!!\x00"
        bad_archives = {}
        for index, name in enumerate(("mk-agent", "mkvsock-relay", "mk_transport.ko")):
            wrong = list(entries)
            wrong[index] = (name, b"wrong embedded component", 0o100755, 0, 1)
            bad_archives["wrong " + name] = archive_bytes(wrong)
        for label, first in [
            ("writable", ("mk-agent", entries[0][1], 0o100775, 0, 1)),
            ("wrong owner", ("mk-agent", entries[0][1], 0o100755, 1000, 1)),
            ("hardlink", ("mk-agent", entries[0][1], 0o100755, 0, 2)),
            ("symlink", ("mk-agent", entries[0][1], 0o120755, 0, 1)),
            ("setuid", ("mk-agent", entries[0][1], 0o104755, 0, 1)),
            ("nonexecutable", ("mk-agent", entries[0][1], 0o100644, 0, 1)),
            ("traversal", ("../mk-agent", entries[0][1], 0o100755, 0, 1)),
        ]:
            bad_archives[label] = archive_bytes([first, *entries[1:]])
        bad_archives.update({
            "aliased trailer": gzip.compress(aliased_trailer, mtime=0),
            "missing": archive_bytes(entries[1:]),
            "duplicate": archive_bytes([*entries, entries[0]]),
            "normalized duplicate": archive_bytes([*entries, ("./mk-agent", *entries[0][1:])]),
            "unsafe root": archive_bytes([*entries, (".", b"", 0o40777, 0, 2)]),
            "second gzip": good_archive + good_archive,
            "encoded tail": good_archive + b"\x00",
            "truncated gzip": good_archive[:-4],
        })
        for name, data in bad_archives.items():
            initramfs.write_bytes(data)
            candidate = copy.deepcopy(manifest)
            candidate["initramfs"]["sha256"] = digest(initramfs)
            path.write_text(json.dumps(candidate))
            result = validate(path)
            if result.returncode == 0 or result.stdout:
                raise AssertionError(f"hash-approved {name} archive accepted: {result.stderr!r}")
        initramfs.write_bytes(good_archive)
        manifest["initramfs"]["sha256"] = digest(initramfs)
        # Inflate across multiple bounded output chunks and reject content after
        # the trailer even when the gzip CRC and whole-archive hash are correct.
        projected = {name: (digest(directory / name), name != "mk_transport.ko")
                     for name in ("mk-agent", "mkvsock-relay", "mk_transport.ko")}
        api["verify_boot_components"](io.BytesIO(good_archive), projected)
        with_padding = gzip.compress(gzip.decompress(good_archive) + bytes(2 << 20), mtime=0)
        api["verify_boot_components"](io.BytesIO(with_padding), projected)
        try:
            api["verify_boot_components"](io.BytesIO(gzip.compress(gzip.decompress(good_archive) + b"x", mtime=0)), projected)
        except ValueError:
            pass
        else:
            raise AssertionError("nonzero decompressed tail accepted")

        path.write_text(json.dumps(manifest), encoding="utf-8")
        for mode in ("missing", "symlink", "writable", "oversized"):
            config.unlink()
            if mode == "symlink":
                target = directory / "config-target"
                target.write_text("CONFIG_MULTIKERNEL=y\n")
                config.symlink_to(target)
            elif mode == "writable":
                config.write_text("CONFIG_MULTIKERNEL=y\n")
                config.chmod(0o666)
            elif mode == "oversized":
                with config.open("wb") as stream:
                    stream.truncate((16 << 20) + 1)
            if validate(path).returncode == 0:
                raise AssertionError(f"{mode} kernel config accepted")
            if mode == "missing":
                config.write_text("CONFIG_MULTIKERNEL=y\n")
        config.unlink()
        config.write_text("CONFIG_MULTIKERNEL=y\n")
        config.chmod(0o644)
        path.chmod(0o622)
        if validate(path).returncode == 0:
            raise AssertionError("writable manifest accepted")
    print("runtime bootstrap validation: PASS (held hashes, kernel ELF release/bounds, mutation, ownership, transport and embedded archive components)")


if __name__ == "__main__":
    main()
