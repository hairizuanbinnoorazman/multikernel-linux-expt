#!/usr/bin/env python3
"""Contract tests for final G4-G6 claim-to-raw-evidence closure."""

import json
import os
from pathlib import Path
import subprocess
import tempfile


REPO = Path(__file__).resolve().parent.parent
SCRIPT = REPO / "scripts/audit-g4-g6-evidence.py"
REQUIREMENTS = REPO / "docs/runtime/contracts/g4-g6-evidence-requirements-v1.json"
NOW = "2026-09-07T01:00:00Z"
LATER = "2026-09-07T02:00:00Z"
INSTANCE = "final-vm"
PROJECT = "test-project"
ZONE = "asia-southeast1-b"
BOOT = "final-vm-boot"
DATA = "retained-data"


def ledger(before):
    disks = [
        {
            "name": DATA,
            "zone": ZONE,
            "status": "READY",
            "size_gb": 20,
            "disk_type": "pd-balanced",
            "labels": {},
            "users": [INSTANCE] if before else [],
            "source_snapshot": None,
        }
    ]
    instances = []
    if before:
        disks.insert(
            0,
            {
                "name": BOOT,
                "zone": ZONE,
                "status": "READY",
                "size_gb": 100,
                "disk_type": "pd-balanced",
                "labels": {},
                "users": [INSTANCE],
                "source_snapshot": "qualified",
            },
        )
        instances.append(
            {
                "name": INSTANCE,
                "zone": ZONE,
                "status": "RUNNING",
                "machine_type": "n2-standard-16",
                "creation_timestamp": NOW,
                "labels": {"disposable": "true"},
                "disks": [
                    {"name": BOOT, "boot": True, "auto_delete": True},
                    {"name": DATA, "boot": False, "auto_delete": False},
                ],
                "external_ips": ["192.0.2.1"],
            }
        )
    return {
        "schema_version": 1,
        "captured_at": NOW if before else LATER,
        "provider": "gce",
        "project": PROJECT,
        "instances": instances,
        "disks": disks,
        "snapshots": [],
        "addresses": [],
        "firewall_rules": [],
    }


def make_bundle(parent, name):
    bundle = parent / name
    bundle.mkdir()
    requirements = json.loads(REQUIREMENTS.read_text(encoding="utf-8"))
    (bundle / "resources-before.json").write_text(
        json.dumps(ledger(True), indent=2) + "\n", encoding="utf-8"
    )
    (bundle / "resources-after.json").write_text(
        json.dumps(ledger(False), indent=2) + "\n", encoding="utf-8"
    )
    components = [
        {"name": component, "version": "1.0.0", "sha256": "a" * 64}
        for component in requirements["required_components"]
    ]
    for gate, assertion_ids in requirements["required_gate_assertions"].items():
        lower = gate.lower()
        argv = [f"run-{lower}", "--record-values"]
        transcript = [
            json.dumps({"evidence_command": {"argv": argv, "started_at": NOW}}, sort_keys=True),
            *(f"EVIDENCE_ASSERTION id={item} passed=true" for item in assertion_ids),
            json.dumps(
                {"evidence_command_result": {"ended_at": LATER, "exit_status": 0}},
                sort_keys=True,
            ),
        ]
        log_name = f"{lower}.log"
        (bundle / log_name).write_text("\n".join(transcript) + "\n", encoding="utf-8")
        manifest = {
            "schema_version": 1,
            "gate": gate,
            "run_id": f"final-{lower}",
            "result": "pass",
            "started_at": NOW,
            "ended_at": LATER,
            "repository": {"commit": "b" * 40, "dirty": False},
            "components": components,
            "host": {
                "provider": "gce",
                "project": PROJECT,
                "zone": ZONE,
                "instance": INSTANCE,
                "machine_type": "n2-standard-16",
                "boot_id": "01234567-89ab-cdef-0123-456789abcdef",
            },
            "commands": [
                {"id": f"run-{lower}", "argv": argv, "exit_status": 0, "output_path": log_name}
            ],
            "assertions": [
                {"id": item, "passed": True, "evidence_paths": [log_name]}
                for item in assertion_ids
            ],
            "limitations": [],
            "cleanup": {
                "completed": True,
                "resources_before": "resources-before.json",
                "resources_after": "resources-after.json",
                "retained_resources": [DATA],
                "operator_acknowledged": True,
            },
        }
        (bundle / f"manifest-{lower}.json").write_text(
            json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
        )
    return bundle


def run(*arguments, ok=True):
    completed = subprocess.run(
        ["python3", os.fspath(SCRIPT), *map(os.fspath, arguments)],
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


def mutate_manifest(bundle, gate, callback):
    path = bundle / f"manifest-{gate.lower()}.json"
    document = json.loads(path.read_text(encoding="utf-8"))
    callback(document)
    path.write_text(json.dumps(document, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main():
    with tempfile.TemporaryDirectory(prefix="g4-g6-audit-test-") as temporary:
        work = Path(temporary)
        valid = make_bundle(work, "valid")
        index = valid / "bundle-sha256.json"
        run("audit", valid, index)
        run("verify", valid, index)
        original_index = index.read_text(encoding="utf-8")
        tampered_index = json.loads(original_index)
        tampered_index["gates"] = ["G4"]
        index.write_text(json.dumps(tampered_index), encoding="utf-8")
        assert "exactly G4, G5, and G6" in run("verify", valid, index, ok=False).stderr
        index.write_text(original_index, encoding="utf-8")
        with (valid / "g4.log").open("a", encoding="utf-8") as output:
            output.write("post-audit mutation\n")
        assert "differ from hash index" in run("verify", valid, index, ok=False).stderr

        missing = make_bundle(work, "missing")
        mutate_manifest(missing, "G5", lambda item: item["assertions"].pop())
        assert "missing required assertions" in run(
            "audit", missing, missing / "index.json", ok=False
        ).stderr

        marker = make_bundle(work, "marker")
        log = marker / "g6.log"
        log.write_text(
            log.read_text(encoding="utf-8").replace(" passed=true", " passed=false", 1),
            encoding="utf-8",
        )
        assert "lacks its raw PASS marker" in run(
            "audit", marker, marker / "index.json", ok=False
        ).stderr

        identity = make_bundle(work, "identity")
        mutate_manifest(
            identity,
            "G5",
            lambda item: item["components"][0].update({"sha256": "c" * 64}),
        )
        assert "component identity differs" in run(
            "audit", identity, identity / "index.json", ok=False
        ).stderr

        cleanup = make_bundle(work, "cleanup")
        (cleanup / "resources-after.json").write_text(
            json.dumps(ledger(True), indent=2) + "\n", encoding="utf-8"
        )
        assert "still contains tested instance" in run(
            "audit", cleanup, cleanup / "index.json", ok=False
        ).stderr

        unsafe = make_bundle(work, "unsafe")
        (unsafe / "g4.log").unlink()
        os.symlink(valid / "g4.log", unsafe / "g4.log")
        assert "unsafe or missing bundle reference" in run(
            "audit", unsafe, unsafe / "index.json", ok=False
        ).stderr

    print("G4-G6 final evidence audit tests: PASS")


if __name__ == "__main__":
    main()
