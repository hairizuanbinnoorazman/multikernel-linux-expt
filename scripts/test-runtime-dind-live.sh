#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
export MK_BUILD_SERVICE=docker
export MK_BUILDKIT_ROOTFS=${MK_DOCKER_ROOTFS:?provide the pinned exported Docker rootfs}
exec bash "$repo/scripts/test-runtime-guest-build-live.sh"
