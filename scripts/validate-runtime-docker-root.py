#!/usr/bin/env python3
"""Bind an approved Docker profile to a held-root scan and safe copied inputs."""
import argparse
import hashlib
import bisect
import json
import os
from pathlib import Path, PurePosixPath
import stat
import sys

LIMIT = 64 << 20

class ApprovalError(Exception):
    pass


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ApprovalError("duplicate metadata field")
        result[key] = value
    return result


def read_metadata(path):
    fd = os.open(path, os.O_RDONLY | os.O_CLOEXEC | os.O_NOFOLLOW | os.O_NONBLOCK)
    try:
        before = os.fstat(fd)
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid()
                or before.st_nlink != 1 or before.st_mode & 0o022
                or not 0 < before.st_size <= LIMIT):
            raise ApprovalError("unsafe generated metadata")
        with os.fdopen(os.dup(fd), "rb") as stream:
            data = stream.read(LIMIT + 1)
        after = os.fstat(fd)
        identity = lambda v: (v.st_dev, v.st_ino, v.st_mode, v.st_uid, v.st_gid,
                              v.st_nlink, v.st_size, v.st_mtime_ns, v.st_ctime_ns)
        if len(data) != before.st_size or identity(before) != identity(after):
            raise ApprovalError("generated metadata changed during read")
        return data
    finally:
        os.close(fd)


def validate(bootstrap, root_bytes, plan):
    profile = bootstrap.get("docker_guest")
    if not isinstance(profile, dict) or profile.get("kind") != "docker-rootless-offline-v1":
        raise ApprovalError("unsupported Docker approval")
    digest = hashlib.sha256(root_bytes).hexdigest()
    if digest != profile.get("rootfs_manifest_sha256"):
        raise ApprovalError("held source root differs from approved image")
    root = json.loads(root_bytes, object_pairs_hook=strict_object)
    if not isinstance(root, dict):
        raise ApprovalError("malformed source manifest")
    entries = root.get("entries")
    if root.get("schema_version") != 1 or not isinstance(entries, list) or len(entries) > 131072:
        raise ApprovalError("malformed source manifest")
    paths = {}
    for entry in entries:
        if not isinstance(entry, dict) or not isinstance(entry.get("path"), str) or entry["path"] in paths:
            raise ApprovalError("malformed source entries")
        path = entry["path"]
        if (path.startswith("/") or "\0" in path or len(path) > 4096
                or str(PurePosixPath(path)) != path or ".." in PurePosixPath(path).parts
                or entry.get("type") not in ("directory", "regular", "symlink")):
            raise ApprovalError("noncanonical source entry")
        paths[entry["path"]] = entry
    aliases = sorted(path for path, entry in paths.items()
                     if entry["type"] == "symlink" or entry.get("hardlink") is not None)
    if not isinstance(plan, dict):
        raise ApprovalError("malformed copied-input plan")
    binds = plan.get("readonly_binds")
    if plan.get("schema_version") != 1 or not isinstance(binds, list) or len(binds) > 256:
        raise ApprovalError("malformed copied-input plan")
    for item in binds:
        destination = item.get("destination") if isinstance(item, dict) else None
        if (not isinstance(destination, str) or not destination.startswith("/")
                or destination.startswith("//") or "\0" in destination
                or len(destination) > 4096 or str(PurePosixPath(destination)) != destination
                or ".." in PurePosixPath(destination).parts):
            raise ApprovalError("noncanonical copied-input destination")
        context = destination == "/work" or destination.startswith("/work/")
        if not context and destination not in ("/etc/hosts", "/etc/hostname", "/etc/resolv.conf"):
            raise ApprovalError("copied input can replace a trusted Docker dependency")
        parts = PurePosixPath(destination).parts[1:]
        for index in range(1, len(parts) + 1):
            entry = paths.get("/".join(parts[:index]))
            if entry is not None and (entry.get("type") == "symlink"
                    or entry.get("hardlink") is not None
                    or index < len(parts) and entry.get("type") != "directory"):
                raise ApprovalError("copied input traverses an image alias")
        prefix = "/".join(parts) + "/"
        index = bisect.bisect_left(aliases, prefix)
        if index < len(aliases) and aliases[index].startswith(prefix):
            raise ApprovalError("copied-input tree contains an image alias")
    return {"schema_version": 1, "source_bound": True,
            "rootfs_manifest_sha256": digest, "copied_input_count": len(binds)}


# The helper closure is parsed as data. Never use ldd, execute image binaries,
# consult the primary loader cache, or link outer code into the writable root.
import struct
import re
from runtime_safe_publish import open_directory_nofollow

HELPERS = {"usr/bin/newuidmap": 0o4755, "usr/bin/newgidmap": 0o4755,
           "usr/local/bin/rootlesskit": 0o755}
LIB_DIRS = ("lib", "usr/local/lib", "usr/lib", "lib64", "usr/lib64",
            "lib/x86_64-linux-gnu", "usr/lib/x86_64-linux-gnu")
INTERPRETERS = {"lib/ld-musl-x86_64.so.1", "lib64/ld-linux-x86-64.so.2",
                "lib/ld-linux-x86-64.so.2", "lib/x86_64-linux-gnu/ld-linux-x86-64.so.2"}


def elf_dependencies(data):
    """Read only bounded ELF64/x86-64 program metadata, never section/tool output."""
    def part(offset, size):
        if offset < 0 or size < 0 or offset > len(data) or size > len(data) - offset:
            raise ApprovalError("helper ELF range exceeds file")
        return data[offset:offset + size]
    if len(data) < 64 or data[:7] != b"\x7fELF\x02\x01\x01":
        raise ApprovalError("helper is not Linux x86-64 ELF")
    header = struct.unpack("<HHIQQQIHHHHHH", part(16, 48))
    kind, machine, version = header[:3]
    phoff, ehsize, phsize, phnum = header[4], header[7], header[8], header[9]
    if kind not in (2, 3) or machine != 62 or version != 1 or ehsize != 64 or phsize != 56 or not 1 <= phnum <= 128:
        raise ApprovalError("unsupported helper ELF headers")
    loads, dynamic, interpreter = [], None, None
    for i in range(phnum):
        tag, _, offset, address, _, filesz, memsz, _ = struct.unpack("<IIQQQQQQ", part(phoff + i * phsize, phsize))
        part(offset, filesz)
        if tag == 1:
            if filesz > memsz:
                raise ApprovalError("helper ELF load size refused")
            loads.append((address, filesz, offset))
        elif tag == 2:
            if dynamic is not None or filesz > 128 << 10 or filesz % 16:
                raise ApprovalError("helper ELF dynamic table refused")
            dynamic = part(offset, filesz)
        elif tag == 3:
            text = part(offset, filesz)
            if interpreter is not None or not 2 <= len(text) <= 4096 or not text.endswith(b"\0") or b"\0" in text[:-1]:
                raise ApprovalError("helper ELF interpreter refused")
            try:
                interpreter = text[:-1].decode("ascii")
            except UnicodeError:
                raise ApprovalError("helper ELF interpreter refused") from None
            if interpreter[1:] not in INTERPRETERS or not interpreter.startswith("/"):
                raise ApprovalError("helper ELF interpreter is outside fixed loader policy")
    if not loads:
        raise ApprovalError("helper ELF has no load segment")
    needed, strings, size = [], None, None
    if dynamic is not None:
        ended = False
        for i in range(0, len(dynamic), 16):
            tag, value = struct.unpack("<qQ", dynamic[i:i + 16])
            if tag == 0:
                ended = True
                break
            if tag in (15, 29, 0x6ffffefa, 0x6ffffefb, 0x6ffffefc, 0x7ffffffd, 0x7fffffff):
                raise ApprovalError("helper ELF custom loader search path refused")
            if tag == 1:
                needed.append(value)
                if len(needed) > 128:
                    raise ApprovalError("helper ELF dependency limit exceeded")
            if tag == 5:
                if strings is not None:
                    raise ApprovalError("helper ELF string table is ambiguous")
                strings = value
            if tag == 10:
                if size is not None:
                    raise ApprovalError("helper ELF string size is ambiguous")
                size = value
        if not ended:
            raise ApprovalError("helper ELF dynamic terminator missing")
    names = []
    if needed:
        if strings is None or size is None or not 0 < size <= 1 << 20:
            raise ApprovalError("helper ELF dependency strings missing")
        tables = [part(offset + strings - address, size) for address, length, offset in loads
                  if address <= strings and strings - address + size <= length]
        if len(tables) != 1:
            raise ApprovalError("helper ELF dependency mapping is ambiguous")
        table = tables[0]
        for offset in needed:
            if offset >= len(table):
                raise ApprovalError("helper ELF dependency name exceeds table")
            end = table.find(b"\0", offset)
            if end < 0 or end - offset > 255:
                raise ApprovalError("helper ELF dependency name is unterminated")
            try:
                name = table[offset:end].decode("ascii")
            except UnicodeError:
                raise ApprovalError("helper ELF dependency name refused") from None
            if re.fullmatch(r"[A-Za-z0-9_.+-]{1,255}", name) is None or name in (".", ".."):
                raise ApprovalError("helper ELF dependency name refused")
            names.append(name)
    return interpreter, names


def resolve_image_path(paths, requested):
    """Resolve links in the approved virtual image, never the primary filesystem."""
    todo, parts, links = requested.split("/"), [], 0
    while todo:
        component, todo = todo[0], todo[1:]
        if component in ("", "."):
            continue
        if component == "..":
            if not parts:
                raise ApprovalError("helper alias escapes image root")
            parts.pop()
            continue
        name = "/".join(parts + [component])
        entry = paths.get(name)
        if entry is None:
            raise ApprovalError("required helper dependency is absent")
        if entry["type"] == "symlink":
            links += 1
            if links > 32:
                raise ApprovalError("helper alias depth exceeded")
            target = entry.get("target")
            if not isinstance(target, str) or not target or "\0" in target or len(target) > 4096:
                raise ApprovalError("helper alias target refused")
            if target.startswith("/"):
                parts = []
            todo = target.split("/") + todo
        else:
            if todo and entry["type"] != "directory":
                raise ApprovalError("helper path traverses non-directory")
            parts.append(component)
    final = "/".join(parts)
    if not final or paths[final]["type"] != "regular":
        raise ApprovalError("helper dependency is not regular")
    return final


def read_image_file(rootfd, paths, requested, required_uid):
    name = resolve_image_path(paths, requested)
    entry = paths[name]
    # Release archives can retain a build UID on rootlesskit. It is not a
    # setuid helper, and its approved UID is checked against the held source.
    # Its independent outer copy is created by root, never by that source UID.
    source_uid = entry.get("uid") if requested == "usr/local/bin/rootlesskit" else required_uid
    if (not isinstance(source_uid, int) or isinstance(source_uid, bool) or not 0 <= source_uid < 1 << 32
            or entry.get("uid") != source_uid or entry.get("mode", 0) & 0o022
            or not isinstance(entry.get("size"), int) or not 0 < entry["size"] <= 64 << 20):
        raise ApprovalError("unsafe approved helper metadata")
    directory = os.dup(rootfd)
    try:
        components = name.split("/")
        for component in components[:-1]:
            fd = os.open(component, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_CLOEXEC, dir_fd=directory)
            os.close(directory)
            directory = fd
        fd = os.open(components[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_CLOEXEC | os.O_NONBLOCK, dir_fd=directory)
    finally:
        os.close(directory)
    try:
        before = os.fstat(fd)
        if (not stat.S_ISREG(before.st_mode) or before.st_uid != entry["uid"]
                or before.st_gid != entry["gid"] or before.st_mode != entry["mode"]
                or before.st_size != entry["size"]):
            raise ApprovalError("held helper metadata differs from approval")
        with os.fdopen(os.dup(fd), "rb") as stream:
            data = stream.read((64 << 20) + 1)
        after = os.fstat(fd)
        identity = lambda s: (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid,
                              s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
        if (len(data) != before.st_size or identity(before) != identity(after)
                or hashlib.sha256(data).hexdigest() != entry.get("sha256")):
            raise ApprovalError("held helper content differs from approval")
        return data
    finally:
        os.close(fd)


def prepare_outer(source, root_bytes, output, required_uid=0):
    """Build a new independent immutable-code closure in caller-private scratch.

    required_uid is an internal fixture seam; the CLI always requires root.
    No copied executable is invoked. Source aliases are flattened to independent
    regular files, so no outer executable/library references the writable root.
    """
    if os.geteuid() != required_uid:
        raise ApprovalError("outer helper preparation requires trusted ownership")
    entries = json.loads(root_bytes, object_pairs_hook=strict_object)["entries"]
    paths = {entry["path"]: entry for entry in entries}
    rootfd = open_directory_nofollow(Path(source))
    files = {}
    pending = list(HELPERS)
    queued = set(pending)
    def enqueue(name):
        if name not in queued:
            if len(queued) >= 256:
                raise ApprovalError("outer helper closure count exceeded")
            queued.add(name)
            pending.append(name)
    size = 0
    try:
        while pending:
            name = pending.pop(0)
            if name in files:
                continue
            if len(files) >= 256:
                raise ApprovalError("outer helper closure count exceeded")
            data = read_image_file(rootfd, paths, name, required_uid)
            size += len(data)
            if size > 128 << 20:
                raise ApprovalError("outer helper closure bytes exceeded")
            interpreter, needed = elf_dependencies(data)
            files[name] = data
            if interpreter:
                enqueue(interpreter[1:])
            for library in needed:
                candidates = []
                for directory in LIB_DIRS:
                    candidate = directory + "/" + library
                    try:
                        resolve_image_path(paths, candidate)
                    except ApprovalError:
                        continue
                    candidates.append(candidate)
                if not candidates:
                    raise ApprovalError("required helper library is absent")
                # Copy every default-search candidate. Their different loader
                # search orders must never fall back to primary libraries.
                for candidate in candidates:
                    enqueue(candidate)
    finally:
        os.close(rootfd)
    # Validate the full closure before allocating its output. The builder owns
    # the private scratch parent; refuse any existing destination, including links.
    output = Path(output)
    output.mkdir(mode=0o700)
    for name, data in files.items():
        path = output / name
        directory = output
        for component in Path(name).parts[:-1]:
            directory /= component
            directory.mkdir(mode=0o755, exist_ok=True)
            directory.chmod(0o755)
        with path.open("xb") as stream:
            stream.write(data)
        path.chmod(HELPERS.get(name, 0o755))
    etc = output / "etc"
    etc.mkdir(mode=0o755)
    etc.chmod(0o755)
    for name, data in {
        "passwd": "root:x:0:0:root:/root:/bin/sh\nrootless:x:1000:1000:rootless:/home/rootless:/bin/sh\n",
        "group": "root:x:0:\nrootless:x:1000:\n",
        "subuid": "rootless:100000:65536\n", "subgid": "rootless:100000:65536\n",
        "nsswitch.conf": "passwd: files\ngroup: files\nsubid: files\n",
    }.items():
        path = etc / name
        with path.open("x") as stream:
            stream.write(data)
        path.chmod(0o644)
    output.chmod(0o755)
    return {"files": len(files), "bytes": size, "independent_code": True,
            "file_sha256": {name: hashlib.sha256(data).hexdigest() for name, data in sorted(files.items())}}


def validate_workload(config):
    process = config.get("process") if isinstance(config, dict) else None
    user = process.get("user") if isinstance(process, dict) else None
    if (not isinstance(user, dict) or type(user.get("uid")) is not int
            or type(user.get("gid")) is not int or user["uid"] != 1000 or user["gid"] != 1000):
        raise ApprovalError("Docker guest workloads require UID and GID 1000")
    groups = user.get("additionalGids", [])
    if not isinstance(groups, list) or any(type(gid) is not int or gid != 1000 for gid in groups):
        raise ApprovalError("Docker guest supplementary group is outside fixed identity")
    if process.get("capabilities") is not None:
        raise ApprovalError("Docker guest workloads may not provide a capability contract")
    if "noNewPrivileges" in process and process["noNewPrivileges"] is not True:
        raise ApprovalError("Docker guest workloads require no-new-privileges")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("bootstrap")
    parser.add_argument("manifest")
    parser.add_argument("bind_plan")
    parser.add_argument("--prepare-outer", nargs=2, metavar=("SOURCE", "OUTPUT"))
    parser.add_argument("--workload-config")
    args = parser.parse_args()
    bootstrap = json.loads(read_metadata(args.bootstrap), object_pairs_hook=strict_object)
    root = read_metadata(args.manifest)
    plan = json.loads(read_metadata(args.bind_plan), object_pairs_hook=strict_object)
    result = validate(bootstrap, root, plan)
    if args.workload_config:
        validate_workload(json.loads(read_metadata(args.workload_config), object_pairs_hook=strict_object))
        result["workload_identity_bound"] = True
    if args.prepare_outer:
        result["outer_helpers"] = prepare_outer(args.prepare_outer[0], root, args.prepare_outer[1])
    print(json.dumps(result, sort_keys=True))

if __name__ == "__main__":
    try:
        main()
    except (ApprovalError, OSError, ValueError, TypeError, AttributeError) as error:
        # Do not reflect host source paths, destinations or malformed payloads.
        message = str(error) if isinstance(error, ApprovalError) else "invalid generated metadata"
        print("Docker root approval rejected: " + message, file=sys.stderr)
        sys.exit(1)
