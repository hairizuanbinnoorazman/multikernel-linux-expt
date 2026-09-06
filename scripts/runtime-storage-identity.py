#!/usr/bin/env python3
"""Derive reproducible ext4 identity from a validated task and source manifest."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import uuid


IDENTITY = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("task_identity")
    parser.add_argument("source_manifest", type=Path)
    arguments = parser.parse_args()
    if not IDENTITY.fullmatch(arguments.task_identity):
        parser.error("task identity is malformed")
    manifest = arguments.source_manifest.read_bytes()
    source_digest = hashlib.sha256(manifest).hexdigest()
    filesystem_uuid = uuid.uuid5(uuid.NAMESPACE_URL, "multikernel-runtime-v1\0" + arguments.task_identity + "\0" + source_digest)
    print(json.dumps({
        "source_manifest_sha256": source_digest,
        "image_id": "root-" + source_digest[:32],
        "filesystem_uuid": str(filesystem_uuid),
    }, separators=(",", ":"), sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
