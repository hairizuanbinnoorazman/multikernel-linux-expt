#!/usr/bin/env bash
set -euo pipefail

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

cd "$kerf_dir"
actual_commit=$(git rev-parse HEAD)
if [[ "$actual_commit" != "$kerf_commit" ]]; then
    echo "unexpected Kerf commit: $actual_commit (wanted $kerf_commit)" >&2
    exit 1
fi

make

# PyPI pylibfdt 1.7.2 does not build on Python 3.14. Ubuntu 26.04's
# python3-libfdt package does, so expose distro packages inside the venv and
# install the pinned Kerf tree without re-resolving pylibfdt from PyPI.
python3 -m venv --clear --system-site-packages .venv
.venv/bin/pip install rdtsc==0.2.1
.venv/bin/pip install --no-deps -e .
.venv/bin/python -c \
    'import click, libfdt, pyudev, rdtsc, yaml; print("kerf dependencies OK")'
.venv/bin/kerf --version
