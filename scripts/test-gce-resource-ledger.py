#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import sys


SCRIPT = Path(__file__).with_name("collect-gce-resource-ledger.py")
previous = sys.dont_write_bytecode
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location("gce_resource_ledger", SCRIPT)
module = importlib.util.module_from_spec(spec)
assert spec.loader
spec.loader.exec_module(module)
sys.dont_write_bytecode = previous

raw = {
    "instances": [{
        "name": "vm", "zone": "zones/z", "status": "RUNNING",
        "machineType": "machineTypes/n2", "creationTimestamp": "2026-09-06T00:00:00Z",
        "labels": {"disposable": "true"},
        "disks": [
            {"source": "zones/z/disks/data", "boot": False, "autoDelete": False},
            {"source": "zones/z/disks/boot", "boot": True, "autoDelete": True},
        ],
        "networkInterfaces": [{"accessConfigs": [{"natIP": "192.0.2.3"}]}],
    }],
    "disks": [{
        "name": "data", "zone": "zones/z", "status": "READY", "sizeGb": "20",
        "type": "diskTypes/pd-balanced", "users": ["zones/z/instances/vm"],
    }],
    "snapshots": [{
        "name": "qualified", "status": "READY", "diskSizeGb": "100", "storageBytes": "42",
        "creationTimestamp": "2026-09-01T00:00:00Z", "sourceDisk": "zones/z/disks/old",
    }],
    "addresses": [],
    "firewall_rules": [{"name": "ssh", "network": "networks/default", "allowed": [{"IPProtocol": "tcp"}]}],
}
ledger = module.normalize("project", "2026-09-06T01:00:00Z", raw)
assert ledger["instances"][0]["disks"] == [
    {"name": "boot", "boot": True, "auto_delete": True},
    {"name": "data", "boot": False, "auto_delete": False},
]
assert ledger["instances"][0]["external_ips"] == ["192.0.2.3"]
assert ledger["disks"][0]["users"] == ["vm"]
assert ledger["disks"][0]["source_snapshot"] is None
assert ledger["snapshots"][0]["source_disk"] == "old"
assert ledger["firewall_rules"][0]["direction"] == "INGRESS"
assert ledger["firewall_rules"][0]["priority"] == 1000
print("GCE resource ledger tests: PASS")
