#!/usr/bin/env python3
"""Run a terminal client, then resize its PTY only after guest readiness."""

import argparse
import fcntl
import os
import select
import signal
import struct
import subprocess
import sys
import termios
import time


def set_size(fd, rows, columns):
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, columns, 0, 0))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--initial-rows", type=int, default=24)
    parser.add_argument("--initial-columns", type=int, default=80)
    parser.add_argument("--resize-rows", type=int, default=37)
    parser.add_argument("--resize-columns", type=int, default=91)
    parser.add_argument("--timeout", type=float, default=30)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command:
        parser.error("a command is required after --")
    master, slave = os.openpty()
    set_size(slave, args.initial_rows, args.initial_columns)

    def child_setup():
        os.setsid()
        fcntl.ioctl(slave, termios.TIOCSCTTY, 0)

    process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, preexec_fn=child_setup, close_fds=True)
    os.close(slave)
    deadline = time.monotonic() + args.timeout
    output = bytearray()
    ready = f"ready:{args.initial_rows} {args.initial_columns}".encode()
    resized = f"resized:{args.resize_rows} {args.resize_columns}".encode()
    resize_sent = False
    try:
        while time.monotonic() < deadline:
            readable, _, _ = select.select([master], [], [], 0.1)
            if readable:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    chunk = b""
                if not chunk:
                    break
                output.extend(chunk)
                sys.stdout.buffer.write(chunk)
                sys.stdout.buffer.flush()
            normalized = bytes(output).replace(b"\r", b"")
            if not resize_sent and ready in normalized:
                set_size(master, args.resize_rows, args.resize_columns)
                os.killpg(process.pid, signal.SIGWINCH)
                resize_sent = True
            if resize_sent and resized in normalized:
                break
            if process.poll() is not None and not readable:
                break
        try:
            status = process.wait(timeout=2)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            status = process.wait()
        normalized = bytes(output).replace(b"\r", b"")
        if ready not in normalized:
            raise RuntimeError(f"initial guest size was not observed: wanted {ready.decode()}")
        if not resize_sent or resized not in normalized:
            raise RuntimeError(f"post-start guest size was not observed: wanted {resized.decode()}")
        if status != 0:
            raise RuntimeError(f"terminal client exited with status {status}")
        print(f"LIVE_RESIZE_PASS initial={args.initial_rows}x{args.initial_columns} resized={args.resize_rows}x{args.resize_columns}")
        return 0
    finally:
        os.close(master)
        if process.poll() is None:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, RuntimeError) as error:
        raise SystemExit(f"live resize failed: {error}")
