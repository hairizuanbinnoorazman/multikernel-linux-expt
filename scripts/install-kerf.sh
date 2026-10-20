#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
# Root-installed Python imports must exclude other directory/file writers.
# Keep development installs unprivileged; production must use root-owned code.
if [[ $EUID = 0 ]]; then umask 022; fi

kerf_dir=${KERF_DIR:-"$HOME/src/kerf"}
kerf_tag=${KERF_TAG:-v0.2.0}
kerf_commit=${KERF_COMMIT:-8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec}

sudo apt-get update
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y \
    device-tree-compiler libfdt-dev musl-tools python3-dev python3-libfdt \
    python3-pyudev python3-venv swig

mkdir -p "$(dirname "$kerf_dir")"
if [[ ! -d "$kerf_dir/.git" ]]; then
    git clone --depth=1 --branch "$kerf_tag" \
        https://github.com/multikernel/kerf.git "$kerf_dir"
fi

kerf_dir=$(cd "$kerf_dir" && pwd -P)
cd "$kerf_dir"
actual_commit=$(git rev-parse HEAD)
if [[ "$actual_commit" != "$kerf_commit" ]]; then
    echo "unexpected Kerf commit: $actual_commit (wanted $kerf_commit)" >&2
    exit 1
fi

if [[ $EUID = 0 ]]; then
    python3 -I -S "$script_dir/harden-kerf-installation.py" --apply "$kerf_dir"
fi

make

# PyPI pylibfdt 1.7.2 does not build on Python 3.14. Ubuntu 26.04's
# python3-libfdt package does, so expose distro packages inside the venv and
# install the pinned Kerf tree without re-resolving pylibfdt from PyPI.
python3 -m venv --clear --system-site-packages .venv
.venv/bin/pip install rdtsc==0.2.1
.venv/bin/pip install --no-deps -e .
if [[ $EUID = 0 ]]; then
    python3 -I -S "$script_dir/harden-kerf-installation.py" --check "$kerf_dir"
fi
.venv/bin/python -c \
    'import click, libfdt, pyudev, rdtsc, yaml; print("kerf dependencies OK")'
.venv/bin/kerf --version
