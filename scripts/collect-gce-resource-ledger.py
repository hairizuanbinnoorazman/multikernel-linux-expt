#!/usr/bin/env python3
"""Collect a complete, normalized GCE project resource ledger."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
import sys


def basename(value):
    return value.rstrip("/").rsplit("/", 1)[-1] if value else None


def ordered(values):
    return sorted(values or [])


def normalize(project, captured_at, raw):
    instances = []
    for value in raw["instances"]:
        external = []
        for interface in value.get("networkInterfaces", []):
            external.extend(
                item["natIP"] for item in interface.get("accessConfigs", []) if item.get("natIP")
            )
        attachments = [
            {
                "name": basename(item.get("source")) or item.get("deviceName"),
                "boot": bool(item.get("boot", False)),
                "auto_delete": bool(item.get("autoDelete", False)),
            }
            for item in value.get("disks", [])
        ]
        instances.append({
            "name": value["name"],
            "zone": basename(value["zone"]),
            "status": value["status"],
            "machine_type": basename(value["machineType"]),
            "creation_timestamp": value["creationTimestamp"],
            "labels": value.get("labels", {}),
            "disks": sorted(attachments, key=lambda item: item["name"]),
            "external_ips": sorted(external),
        })

    disks = [{
        "name": value["name"],
        "zone": basename(value["zone"]),
        "status": value["status"],
        "size_gb": int(value["sizeGb"]),
        "disk_type": basename(value["type"]),
        "labels": value.get("labels", {}),
        "users": sorted(filter(None, (basename(item) for item in value.get("users", [])))),
        "source_snapshot": basename(value.get("sourceSnapshot")),
    } for value in raw["disks"]]

    snapshots = [{
        "name": value["name"],
        "status": value["status"],
        "size_gb": int(value["diskSizeGb"]),
        "storage_bytes": int(value.get("storageBytes", 0)),
        "creation_timestamp": value["creationTimestamp"],
        "labels": value.get("labels", {}),
        "source_disk": basename(value.get("sourceDisk")),
    } for value in raw["snapshots"]]

    addresses = [{
        "name": value["name"],
        "region": basename(value.get("region")),
        "status": value["status"],
        "address": value.get("address", ""),
        "address_type": value.get("addressType", "EXTERNAL"),
        "labels": value.get("labels", {}),
        "users": sorted(filter(None, (basename(item) for item in value.get("users", [])))),
    } for value in raw["addresses"]]

    firewalls = [{
        "name": value["name"],
        "network": basename(value["network"]),
        "direction": value.get("direction", "INGRESS"),
        "priority": int(value.get("priority", 1000)),
        "disabled": bool(value.get("disabled", False)),
        "source_ranges": ordered(value.get("sourceRanges")),
        "destination_ranges": ordered(value.get("destinationRanges")),
        "target_tags": ordered(value.get("targetTags")),
        "allowed": value.get("allowed", []),
        "denied": value.get("denied", []),
    } for value in raw["firewall_rules"]]

    return {
        "schema_version": 1,
        "captured_at": captured_at,
        "provider": "gce",
        "project": project,
        "instances": sorted(instances, key=lambda item: (item["zone"], item["name"])),
        "disks": sorted(disks, key=lambda item: (item["zone"], item["name"])),
        "snapshots": sorted(snapshots, key=lambda item: item["name"]),
        "addresses": sorted(addresses, key=lambda item: ((item["region"] or ""), item["name"])),
        "firewall_rules": sorted(firewalls, key=lambda item: item["name"]),
    }


def query(project, resource):
    completed = subprocess.run(
        ["gcloud", "compute", resource, "list", f"--project={project}", "--format=json"],
        check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    if completed.returncode != 0:
        raise RuntimeError(f"gcloud compute {resource} list failed: {completed.stderr.strip()}")
    return json.loads(completed.stdout)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("project")
    parser.add_argument("output", type=Path)
    arguments = parser.parse_args()
    try:
        if not arguments.project or any(character.isspace() for character in arguments.project):
            raise ValueError("project must be a non-empty token")
        raw = {
            key: query(arguments.project, resource)
            for key, resource in (
                ("instances", "instances"), ("disks", "disks"),
                ("snapshots", "snapshots"), ("addresses", "addresses"),
                ("firewall_rules", "firewall-rules"),
            )
        }
        captured = datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
        payload = (json.dumps(normalize(arguments.project, captured, raw), indent=2, sort_keys=True) + "\n").encode()
        descriptor = os.open(arguments.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(payload)
            stream.flush()
            os.fsync(stream.fileno())
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError) as error:
        print(f"collect GCE resource ledger: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
