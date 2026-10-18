#!/usr/bin/env python3
"""Keep evidence and staged files over 1 MiB in LFS without restaging worktree edits."""
import argparse
import io
import os
from pathlib import Path
import re
import subprocess
import sys

LIMIT = 1 << 20
POINTER = re.compile(rb'version https://git-lfs.github.com/spec/v1\noid sha256:[0-9a-f]{64}\nsize ([0-9]+)\n')


def git(*args, data=None):
    return subprocess.run(['git', *args], input=data, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, check=True).stdout


def entries():
    rows = {}
    for record in git('ls-files', '--stage', '-z').split(b'\0'):
        if not record:
            continue
        metadata, name = record.split(b'\t', 1)
        mode, oid, stage = metadata.split()
        if stage != b'0':
            raise RuntimeError('resolve index conflicts before automatic LFS tracking')
        if mode in (b'100644', b'100755'):
            rows[os.fsdecode(name)] = (mode.decode(), oid.decode())
    return rows


def candidates(all_evidence=False):
    index = entries()
    changed = {os.fsdecode(p) for p in git('diff', '--cached', '--name-only', '--diff-filter=ACMRT', '-z').split(b'\0') if p}
    ids = sorted({oid for mode, oid in index.values()})
    sizes = {}
    if ids:
        output = git('cat-file', '--batch-check=%(objectname) %(objectsize)',
                     data=('\n'.join(ids) + '\n').encode())
        sizes = {oid.decode(): int(size) for oid, size in (line.split() for line in output.splitlines())}
    small_ids = sorted({oid for name, (mode, oid) in index.items()
                        if name.startswith('evidence/') and (all_evidence or name in changed)
                        and sizes[oid] <= 1024})
    small = {}
    if small_ids:
        batch = io.BytesIO(git('cat-file', '--batch', data=('\n'.join(small_ids) + '\n').encode()))
        for oid in small_ids:
            header = batch.readline().split()
            if len(header) != 3 or header[1] != b'blob':
                raise RuntimeError('invalid index object')
            small[oid] = batch.read(int(header[2]))
            if batch.read(1) != b'\n':
                raise RuntimeError('invalid Git object framing')
    result = {}
    for name, (mode, oid) in index.items():
        size = sizes[oid]
        # Git LFS deliberately represents empty files as ordinary empty blobs.
        if size == 0:
            continue
        if size > LIMIT:
            result[name] = (mode, oid, size)
        elif name.startswith('evidence/') and (all_evidence or name in changed):
            raw = small.get(oid, b'')
            if not POINTER.fullmatch(raw):
                result[name] = (mode, oid, size)
    return result


def filter_for(name):
    return git('check-attr', '--cached', '-z', 'filter', '--', name).split(b'\0')[2]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true', help='report policy violations without changing the index')
    args = parser.parse_args()
    os.chdir(os.fsdecode(git('rev-parse', '--show-toplevel').strip()))
    pending = candidates(all_evidence=args.check)
    if not pending:
        return 0
    if args.check:
        for name, (_, _, size) in pending.items():
            print(f'LFS policy violation: {name} ({size} bytes)', file=sys.stderr)
        return 1
    git('lfs', 'version')
    new_rules = []
    for name in pending:
        selected = filter_for(name)
        if selected == b'lfs':
            continue
        if selected != b'unspecified':
            raise RuntimeError(f'refusing to replace an explicit non-LFS filter for {name}')
        new_rules.append(name)
    if new_rules:
        # Attribute changes are staged separately; never absorb unstaged user
        # edits into a commit while adding the new literal filename rules.
        if subprocess.run(['git', 'diff', '--quiet', '--', '.gitattributes']).returncode:
            raise RuntimeError('stage or save unstaged .gitattributes edits before adding new LFS rules')
        if Path('.gitattributes').is_symlink():
            raise RuntimeError('.gitattributes must be a regular file')
        for name in new_rules:
            git('lfs', 'track', '--filename', '--', name)
        git('add', '--', '.gitattributes')
    for name in pending:
        if filter_for(name) != b'lfs':
            raise RuntimeError(f'nested attributes prevent LFS tracking for {name}')
    for name, (mode, oid, size) in pending.items():
        # Convert immutable index bytes, not the worktree file. This preserves
        # partially staged changes and executable modes, even for large text.
        original = subprocess.Popen(['git', 'cat-file', 'blob', oid], stdout=subprocess.PIPE)
        try:
            clean = subprocess.run(['git', 'lfs', 'clean', '--', name], stdin=original.stdout,
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        finally:
            original.stdout.close()
        original_status = original.wait()
        if original_status or clean.returncode:
            raise RuntimeError(f'LFS clean failed for {name}: {clean.stderr.decode(errors="replace")}')
        match = POINTER.fullmatch(clean.stdout)
        if not match or int(match[1]) != size:
            raise RuntimeError(f'LFS returned an invalid pointer for {name}')
        pointer_oid = git('hash-object', '-w', '--stdin', data=clean.stdout).strip().decode()
        if entries().get(name) != (mode, oid):
            raise RuntimeError(f'index changed concurrently for {name}; retry the commit')
        git('update-index', '--cacheinfo', mode, pointer_oid, name)
    if candidates():
        raise RuntimeError('staged files still violate LFS policy')
    print(f'Git LFS: automatically tracked {len(pending)} staged file(s).')
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f'automatic Git LFS tracking failed: {error}', file=sys.stderr)
        sys.exit(1)
