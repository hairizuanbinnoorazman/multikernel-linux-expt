#!/usr/bin/env python3
import pathlib
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory() as directory:
    binary = pathlib.Path(directory) / 'console-check'
    subprocess.run(['cc', '-O2', '-Wall', '-Wextra', '-Werror',
                    str(root/'scripts/test-native-client-console.c'), '-o', str(binary)], check=True)
    subprocess.run([str(binary)], check=True, timeout=5)
