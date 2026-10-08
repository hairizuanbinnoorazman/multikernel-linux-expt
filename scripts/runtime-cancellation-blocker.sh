#!/usr/bin/env bash
set -euo pipefail

# Disposable-VM qualification adapter. The supervising qualifier supplies an
# exact real executable, operation, and private observation directory.
real=${MK_CANCEL_REAL:?MK_CANCEL_REAL is required}
operation=${MK_CANCEL_OPERATION:?MK_CANCEL_OPERATION is required}
control=${MK_CANCEL_CONTROL:?MK_CANCEL_CONTROL is required}

case "$operation" in
	rootfs-build)
		if [[ ${MK_VALIDATE_ONLY:-0} = 1 ]]; then exec "$real" "$@"; fi
		;;
	kerf-load)
		[[ ${1:-} = load ]] || exec "$real" "$@"
		;;
	*) echo "unsupported cancellation operation" >&2; exit 2 ;;
esac

mkdir -p "$control"
chmod 0700 "$control"
printf '%s\n' "$$" >"$control/parent"
(
	trap '' TERM INT
	while :; do sleep 1; done
) &
child=$!
printf '%s\n' "$child" >"$control/child"
printf 'ready\n' >"$control/ready"
wait "$child"
