#!/usr/bin/env python3
"""Stream one command to the terminal and an exclusive evidence transcript."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import subprocess
import sys


SENSITIVE = re.compile(r"(?i)(?:token|password|secret|credential|private[-_]?key)")


def timestamp():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def display_argv(arguments):
    result = []
    redact_next = False
    for argument in arguments:
        if redact_next:
            result.append("[REDACTED]")
            redact_next = False
            continue
        if argument.startswith("-") and SENSITIVE.search(argument):
            if "=" in argument:
                result.append(argument.split("=", 1)[0] + "=[REDACTED]")
            else:
                result.append(argument)
                redact_next = True
        else:
            result.append(argument)
    return result


def emit(stream, data):
    stream.write(data)
    stream.flush()
    sys.stdout.buffer.write(data)
    sys.stdout.buffer.flush()


def capture(output: Path, command):
    descriptor = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, "wb", buffering=0) as transcript:
        header = {
            "evidence_command": {
                "argv": display_argv(command),
                "started_at": timestamp(),
            }
        }
        emit(transcript, (json.dumps(header, sort_keys=True) + "\n").encode())
        try:
            process = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
            assert process.stdout
            # os.read returns currently available pipe data instead of waiting
            # for a full buffered-reader request, keeping long live runs visible.
            for chunk in iter(lambda: os.read(process.stdout.fileno(), 65536), b""):
                emit(transcript, chunk)
            status = process.wait()
        except OSError as error:
            emit(transcript, f"capture runner failed to execute command: {error}\n".encode())
            status = 127
        trailer = {
            "evidence_command_result": {
                "ended_at": timestamp(),
                "exit_status": status,
            }
        }
        emit(transcript, (json.dumps(trailer, sort_keys=True) + "\n").encode())
        os.fsync(transcript.fileno())
    return status


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    arguments = parser.parse_args()
    command = arguments.command
    if command[:1] == ["--"]:
        command = command[1:]
    if not command:
        parser.error("a command is required after the output path")
    try:
        return capture(arguments.output, command)
    except OSError as error:
        print(f"capture evidence command: {error}", file=sys.stderr)
        return 125


if __name__ == "__main__":
    raise SystemExit(main())
