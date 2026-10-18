#!/usr/bin/env python3
"""Validate and resolve runtime-owned bootstrap artifacts from an approved manifest."""

import argparse
import contextlib
import hashlib
import json
import os
import pathlib
import posixpath
import re
import stat
import struct
import subprocess
import sys
import zlib


PINNED_MULTIKERNEL = "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7"
PINNED_KERF = "8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec"


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON field {key!r}")
        result[key] = value
    return result


def exact_fields(value, required, optional=()):
    if not isinstance(value, dict):
        raise ValueError("manifest object expected")
    fields = set(value)
    required = set(required)
    allowed = required | set(optional)
    if fields - allowed or required - fields:
        raise ValueError(f"manifest fields mismatch: missing={sorted(required-fields)} unknown={sorted(fields-allowed)}")


def file_identity(info):
    return (info.st_dev, info.st_ino, info.st_mode, info.st_uid, info.st_gid,
            info.st_size, info.st_mtime_ns, info.st_ctime_ns)


@contextlib.contextmanager
def verified_file(path, digest, required_uid, skip_parent_safety=False, maximum=16 << 30, allow_unhashed=False):
    candidate = pathlib.Path(path)
    if not candidate.is_absolute():
        raise ValueError(f"artifact path is not absolute: {path}")
    if not skip_parent_safety:
        current = pathlib.Path("/")
        for component in candidate.parent.parts[1:]:
            current /= component
            info = current.lstat()
            if stat.S_ISLNK(info.st_mode) or not stat.S_ISDIR(info.st_mode):
                raise ValueError(f"unsafe artifact parent: {current}")
            if info.st_uid not in (0, required_uid) or info.st_mode & 0o022:
                raise ValueError(f"artifact parent has unsafe owner or mode: {current}")
    before = candidate.lstat()
    if (not stat.S_ISREG(before.st_mode) or before.st_uid != required_uid
            or before.st_mode & 0o022 or not 0 < before.st_size <= maximum):
        raise ValueError(f"artifact has unsafe type, owner, mode, or size: {path}")
    if not (allow_unhashed and digest is None) and (not isinstance(digest, str) or re.fullmatch(r"[a-f0-9]{64}", digest) is None):
        raise ValueError(f"artifact digest is malformed: {path}")
    descriptor = os.open(candidate, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        if file_identity(before) != file_identity(os.fstat(stream.fileno())):
            raise ValueError("artifact identity changed while opening")
        if digest is not None:
            actual = hashlib.sha256()
            total = 0
            while chunk := stream.read(min(1 << 20, maximum + 1 - total)):
                actual.update(chunk)
                total += len(chunk)
                if total > maximum:
                    raise ValueError("artifact exceeds bounded read")
            if actual.hexdigest() != digest:
                raise ValueError(f"artifact digest mismatch: {path}")
            stream.seek(0)
        if file_identity(before) != file_identity(os.fstat(stream.fileno())):
            raise ValueError("artifact changed while hashing")
        try:
            yield stream
        finally:
            if file_identity(before) != file_identity(os.fstat(stream.fileno())):
                raise ValueError("artifact changed during verified read")


def secure_regular(path, digest, required_uid, skip_parent_safety=False):
    with verified_file(path, digest, required_uid, skip_parent_safety):
        pass


def artifact(value, required_uid, skip_parent_safety=False):
    exact_fields(value, ["path", "sha256"])
    secure_regular(value["path"], value["sha256"], required_uid, skip_parent_safety)
    return value["path"]


def x86_64_elf(value, required_uid, skip_parent_safety=False):
    path = value["path"]
    with verified_file(path, value["sha256"], required_uid, skip_parent_safety) as stream:
        header = stream.read(20)
    if len(header) < 20 or header[:5] != b"\x7fELF\x02" or int.from_bytes(header[18:20], "little") != 62:
        raise ValueError(f"artifact is not an x86-64 ELF image: {path}")


def kernel_identity(value, release, required_uid, skip_parent_safety=False):
    # The same descriptor is hash-verified, parsed and checked for mutation.
    with verified_file(value["path"], value["sha256"], required_uid, skip_parent_safety) as stream:
        size = os.fstat(stream.fileno()).st_size

        def read(offset, length):
            if offset < 0 or length < 0 or offset > size or length > size - offset:
                raise ValueError("kernel ELF range is outside the file")
            stream.seek(offset)
            data = stream.read(length)
            if len(data) != length:
                raise ValueError("kernel ELF range is truncated")
            return data

        header = read(0, 64)
        if (header[:7] != b"\x7fELF\x02\x01\x01"
                or struct.unpack_from("<HHI", header, 16) != (2, 62, 1)
                or struct.unpack_from("<H", header, 52)[0] != 64):
            raise ValueError("kernel ELF architecture or type is incompatible")
        section_offset = struct.unpack_from("<Q", header, 40)[0]
        section_size, section_count = struct.unpack_from("<HH", header, 58)
        # Canonical vmlinux uses ordinary section numbering. Refuse extended
        # numbering rather than introducing another unbounded parser route.
        if section_size != 64 or not 0 < section_count < 0xff00:
            raise ValueError("kernel section table is incompatible")
        raw = read(section_offset, section_count * 64)
        sections = [struct.unpack_from("<IIQQQQIIQQ", raw, i * 64) for i in range(section_count)]
        name_index = struct.unpack_from("<H", header, 62)[0]
        if not 0 < name_index < section_count:
            raise ValueError("kernel section names are incompatible")
        section_names = sections[name_index]
        if (section_names[1] != 3 or section_names[2] & 0x800
                or not 0 < section_names[5] <= 32 << 20):
            raise ValueError("kernel section names are incompatible")
        section_strings = read(section_names[4], section_names[5])
        for section in sections:
            name = section[0]
            if (name >= len(section_strings)
                    or section_strings.find(b"\x00", name, min(len(section_strings), name + 4096)) < 0):
                raise ValueError("kernel section name is out of bounds")
        program_size, program_count = struct.unpack_from("<HH", header, 54)
        if program_count == 0xffff or (program_count and program_size != 56):
            raise ValueError("kernel program table is incompatible")
        if program_count:
            read(struct.unpack_from("<Q", header, 32)[0], program_count * 56)
        tables = [section for section in sections if section[1] == 2]
        if len(tables) != 1:
            raise ValueError("kernel symbol table is missing or ambiguous")
        table = tables[0]
        if (table[2] & 0x800 or table[9] != 24 or not 0 < table[5] <= 32 << 20
                or table[5] % 24 or table[6] >= section_count):
            raise ValueError("kernel symbol table has unsafe type or size")
        names = sections[table[6]]
        if names[1] != 3 or names[2] & 0x800 or not 0 < names[5] <= 32 << 20:
            raise ValueError("kernel symbol names have unsafe type or size")
        symbols = read(table[4], table[5])
        strings = read(names[4], names[5])
        if symbols[:24] != bytes(24):
            raise ValueError("kernel reserved symbol is incompatible")
        banners = []
        for i in range(24, len(symbols), 24):
            name, info, other, section_index, address, length = struct.unpack_from("<IBBHQQ", symbols, i)
            if name >= len(strings) or strings.find(b"\x00", name, min(len(strings), name + 4096)) < 0:
                raise ValueError("kernel symbol name is out of bounds")
            if strings[name:name + 13] == b"linux_banner\x00":
                banners.append((info, section_index, address, length))
        if len(banners) != 1:
            raise ValueError("kernel release symbol is missing or ambiguous")
        info, index, address, length = banners[0]
        if info != 0x11 or not 0 < index < min(section_count, 0xff00) or not 0 < length <= 4096:
            raise ValueError("kernel release symbol is incompatible")
        section = sections[index]
        if (section[1] != 1 or not section[2] & 2 or section[2] & 0x800
                or address < section[3] or address - section[3] > section[5]
                or length > section[5] - (address - section[3])):
            raise ValueError("kernel release section or symbol range is incompatible")
        if section[4] > size or section[5] > size - section[4]:
            raise ValueError("kernel release section is outside its file")
        banner = read(section[4] + address - section[3], length)
        if (not banner.endswith(b"\x00") or b"\x00" in banner[:-1]
                or not banner.startswith(b"Linux version " + release.encode("ascii") + b" ")):
            raise ValueError("kernel embedded release is incompatible")


def kernel_config(kernel, release, required, required_uid):
    if not isinstance(release, str) or re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", release) is None:
        raise ValueError("kernel release is not a safe filename component")
    if not isinstance(required, list) or any(not isinstance(line, str) for line in required):
        raise ValueError("required kernel config must be a list of strings")
    # The kernel's parent hierarchy was already checked by artifact(). Hold a
    # no-follow descriptor so config verification cannot switch to a symlink.
    path = pathlib.Path(kernel).parent / ("config-" + release)
    descriptor = os.open(path, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as stream:
        before = os.fstat(stream.fileno())
        maximum = 16 << 20
        if not stat.S_ISREG(before.st_mode) or before.st_uid != required_uid or before.st_mode & 0o022 or not 0 < before.st_size <= maximum:
            raise ValueError("kernel config has unsafe type, owner, mode, or size")
        data = stream.read(maximum + 1)
        after = os.fstat(stream.fileno())
        identity = lambda info: (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns)
        if len(data) > maximum or identity(before) != identity(after):
            raise ValueError("kernel config changed during bounded verification")
    lines = {line.strip() for line in data.decode("utf-8").splitlines()}
    for line in required:
        if line not in lines:
            raise ValueError(f"kernel config is missing {line}")



def verify_boot_components(source, required):
    """Read one bounded gzip/newc archive without extraction or implicit members."""
    maximum = 512 << 20

    def chunks():
        decoder = zlib.decompressobj(16 + zlib.MAX_WBITS)
        pending = b""
        total = 0
        while not decoder.eof:
            if not pending:
                pending = source.read(64 << 10)
            try:
                chunk = decoder.decompress(pending, 1 << 20)
            except zlib.error as error:
                raise ValueError("invalid boot archive gzip") from error
            pending = decoder.unconsumed_tail
            total += len(chunk)
            if total > maximum:
                raise ValueError("boot archive exceeds decompressed limit")
            if chunk:
                yield chunk
            elif not pending and not decoder.eof:
                # No output can also mean a consumed header; distinguish that
                # from an exhausted source without accumulating encoded bytes.
                pending = source.read(64 << 10)
                if not pending:
                    raise ValueError("truncated boot archive gzip")
        if decoder.unused_data or pending or source.read(1):
            raise ValueError("boot archive has trailing encoded data")

    blocks = iter(chunks())
    buffered = b""

    def read(length):
        nonlocal buffered
        if not 0 <= length <= 1 << 20:
            raise ValueError("boot archive read exceeds bound")
        result = bytearray()
        while len(result) < length:
            if not buffered:
                try:
                    buffered = next(blocks)
                except StopIteration:
                    raise ValueError("truncated boot archive record") from None
            count = min(length - len(result), len(buffered))
            result.extend(buffered[:count])
            buffered = buffered[count:]
        return bytes(result)

    def padding(length):
        if any(read(length)):
            raise ValueError("nonzero boot archive alignment")

    seen = {}
    found = set()
    for _ in range(131072):
        header = read(110)
        if header[:6] != b"070701" or re.fullmatch(rb"[a-fA-F0-9]{104}", header[6:]) is None:
            raise ValueError("boot archive must use valid newc without CRC")
        fields = [int(header[6 + i * 8:14 + i * 8], 16) for i in range(13)]
        mode, uid, gid, links = fields[1:5]
        size, namesize = fields[6], fields[11]
        if not 0 < namesize <= 4096 or size > maximum or fields[12] != 0:
            raise ValueError("invalid boot archive name, size or checksum")
        raw = read(namesize)
        if raw[-1:] != b"\x00" or b"\x00" in raw[:-1]:
            raise ValueError("invalid boot archive name")
        try:
            name = raw[:-1].decode("utf-8").removeprefix("./")
        except UnicodeDecodeError as error:
            raise ValueError("invalid boot archive UTF-8 name") from error
        padding((-(110 + namesize)) % 4)
        if name == "TRAILER!!!":
            if raw[:-1] != b"TRAILER!!!":
                raise ValueError("noncanonical boot archive trailer")
            if size:
                raise ValueError("boot archive trailer has data")
            if any(buffered):
                raise ValueError("boot archive has data after trailer")
            for chunk in blocks:
                if any(chunk):
                    raise ValueError("boot archive has data after trailer")
            break
        if (not name or name == ".." or name.startswith(("/", "../"))
                or "\\" in name or posixpath.normpath(name) != name):
            raise ValueError("unsafe boot archive path")
        if name in seen:
            raise ValueError("duplicate normalized boot archive path")
        seen[name] = (mode, uid, gid)
        critical = required.get(name)
        if critical and (stat.S_IFMT(mode) != stat.S_IFREG or mode & 0o7022
                         or uid != 0 or gid != 0 or links != 1 or not mode & 0o400
                         or (critical[1] and not mode & 0o100)):
            raise ValueError("boot component has unsafe metadata: " + name)
        digest = hashlib.sha256()
        remaining = size
        while remaining:
            data = read(min(remaining, 1 << 20))
            if critical:
                digest.update(data)
            remaining -= len(data)
        if critical:
            if digest.hexdigest() != critical[0]:
                raise ValueError("boot component digest differs from approved artifact: " + name)
            found.add(name)
        padding((-size) % 4)
    else:
        raise ValueError("boot archive entry limit")
    if found != set(required):
        raise ValueError("boot archive lacks required components")
    for name in found:
        parent = posixpath.dirname(name) or "."
        while True:
            entry = seen.get(parent)
            if (entry is None and parent != ".") or (entry is not None and
                    (stat.S_IFMT(entry[0]) != stat.S_IFDIR or entry[0] & 0o7022
                     or entry[1] != 0 or entry[2] != 0)):
                raise ValueError("boot component has unsafe ancestor: " + name)
            if parent == ".":
                break
            parent = posixpath.dirname(parent) or "."


def initramfs_identity(manifest, required_uid, skip_parent_safety=False):
    required = {
        "mk-agent": (manifest["agent"]["sha256"], True),
        "mkvsock-relay": (manifest["relay"]["sha256"], True),
        "mk_transport.ko": (manifest["transport"]["module"]["sha256"], False),
    }
    value = manifest["initramfs"]
    with verified_file(value["path"], value["sha256"], required_uid,
                       skip_parent_safety, (512 << 20) + (1 << 20)) as stream:
        verify_boot_components(stream, required)

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("manifest")
    parser.add_argument("expected_name")
    parser.add_argument("--required-uid", type=int, default=0)
    parser.add_argument("--skip-modinfo", action="store_true")
    parser.add_argument("--skip-parent-safety", action="store_true")
    args = parser.parse_args()
    manifest_path = pathlib.Path(args.manifest)
    with verified_file(manifest_path, None, args.required_uid, args.skip_parent_safety, 1 << 20, allow_unhashed=True) as stream:
        manifest_bytes = stream.read((1 << 20) + 1)
        if len(manifest_bytes) > 1 << 20:
            raise ValueError("manifest exceeds bounded read")
    manifest = json.loads(manifest_bytes, object_pairs_hook=strict_object)
    exact_fields(
        manifest,
        ["schema_version", "name", "architecture", "kernel_release", "compatibility", "kernel", "initramfs", "agent", "relay", "transport", "protocol", "required_config", "oci_features"],
        ["modules"],
    )
    if manifest["schema_version"] != 1 or manifest["name"] != args.expected_name or manifest["architecture"] != "amd64":
        raise ValueError("manifest identity or architecture is incompatible")
    compatibility = manifest["compatibility"]
    exact_fields(compatibility, ["multikernel_revision", "kerf_version", "kerf_revision"])
    if compatibility != {"multikernel_revision": PINNED_MULTIKERNEL, "kerf_version": "v0.2.0", "kerf_revision": PINNED_KERF}:
        raise ValueError("manifest compatibility pins are incompatible")
    protocol = manifest["protocol"]
    exact_fields(protocol, ["min", "max"])
    if protocol["min"] > 1 or protocol["max"] < 1:
        raise ValueError("manifest does not support protocol v1")

    kernel = artifact(manifest["kernel"], args.required_uid, args.skip_parent_safety)
    kernel_config(kernel, manifest["kernel_release"], manifest["required_config"], args.required_uid)
    kernel_identity(manifest["kernel"], manifest["kernel_release"], args.required_uid, args.skip_parent_safety)
    initramfs = artifact(manifest["initramfs"], args.required_uid, args.skip_parent_safety)
    agent = artifact(manifest["agent"], args.required_uid, args.skip_parent_safety)
    relay = artifact(manifest["relay"], args.required_uid, args.skip_parent_safety)
    transport = manifest["transport"]
    exact_fields(transport, ["module", "module_name", "socket_option", "transport_id", "primary_role", "child_role"])
    expected_transport = {"module_name": "mk_transport", "socket_option": 9, "transport_id": 1, "primary_role": "server", "child_role": "client"}
    if {key: transport[key] for key in expected_transport} != expected_transport:
        raise ValueError("transport module, socket option, ID, or direction is incompatible")
    module = artifact(transport["module"], args.required_uid, args.skip_parent_safety)
    modules = [module] + [artifact(extra, args.required_uid, args.skip_parent_safety) for extra in manifest.get("modules", [])]
    for executable in (manifest["agent"], manifest["relay"]):
        x86_64_elf(executable, args.required_uid, args.skip_parent_safety)
    if not args.skip_modinfo:
        for selected in modules:
            name = subprocess.check_output(["modinfo", "-F", "name", selected], text=True, timeout=10).strip()
            vermagic = subprocess.check_output(["modinfo", "-F", "vermagic", selected], text=True, timeout=10).strip()
            if (selected == module and name != "mk_transport") or not name or "\n" in name or "\n" in vermagic or not vermagic.startswith(manifest["kernel_release"] + " "):
                raise ValueError("module name or kernel release is incompatible")
    initramfs_identity(manifest, args.required_uid, args.skip_parent_safety)
    print(json.dumps({
        "manifest": str(manifest_path),
        "manifest_sha256": hashlib.sha256(manifest_bytes).hexdigest(),
        "architecture": manifest["architecture"],
        "kernel_release": manifest["kernel_release"],
        "required_config": manifest["required_config"],
        "oci_features": manifest["oci_features"],
        "kernel": kernel,
        "initramfs": initramfs,
        "agent": agent,
        "relay": relay,
        "module": module,
    }, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, json.JSONDecodeError, subprocess.SubprocessError) as error:
        print(f"bootstrap validation failed: {error}", file=sys.stderr)
        raise SystemExit(1)
