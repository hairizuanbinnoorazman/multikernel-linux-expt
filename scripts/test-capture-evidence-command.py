#!/usr/bin/env python3
from pathlib import Path
import subprocess
import tempfile


SCRIPT = Path(__file__).with_name("capture-evidence-command.py")
with tempfile.TemporaryDirectory() as raw:
    output = Path(raw) / "command.log"
    completed = subprocess.run(
        [str(SCRIPT), str(output), "--", "/bin/sh", "-c", "echo stdout; echo stderr >&2; exit 17"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    assert completed.returncode == 17
    assert completed.stderr == ""
    assert "stdout\nstderr\n" in completed.stdout
    assert '"exit_status": 17' in completed.stdout
    assert output.read_text() == completed.stdout
    assert output.stat().st_mode & 0o777 == 0o600
    duplicate = subprocess.run(
        [str(SCRIPT), str(output), "--", "/bin/true"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    assert duplicate.returncode == 125
    assert "File exists" in duplicate.stderr
    secret = Path(raw) / "secret.log"
    completed = subprocess.run(
        [str(SCRIPT), str(secret), "--", "/bin/true", "--token-hex", "sensitive", "--password=value"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    assert completed.returncode == 0
    value = secret.read_text()
    assert "sensitive" not in value and "password=value" not in value
    assert value.count("[REDACTED]") == 2
print("evidence command capture tests: PASS")
