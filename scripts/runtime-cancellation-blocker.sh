#!/usr/bin/env bash
set -euo pipefail

# Disposable-VM qualification adapter. The supervising qualifier supplies an
# exact real executable, operation, and private observation directory. Host
# qualification deliberately sanitizes the child environment, so accept the
# same values from a private, root-owned file when the environment is absent.
config=${MK_CANCEL_CONFIG:-/opt/mkruntime/qualification-cancel/blocker.config}
if [[ -n ${MK_CANCEL_REAL:-} && -n ${MK_CANCEL_OPERATION:-} && -n ${MK_CANCEL_CONTROL:-} ]]; then
	real=$MK_CANCEL_REAL
	operation=$MK_CANCEL_OPERATION
	control=$MK_CANCEL_CONTROL
else
	[[ -f $config && ! -L $config ]] || { echo "invalid cancellation blocker config" >&2; exit 2; }
	read -r config_uid config_mode < <(stat -c '%u %a' -- "$config")
	[[ $config_uid = 0 && $config_mode = 600 ]] || { echo "unsafe cancellation blocker config" >&2; exit 2; }
	mapfile -t config_values <"$config"
	[[ ${#config_values[@]} = 3 ]] || { echo "malformed cancellation blocker config" >&2; exit 2; }
	real=${config_values[0]}
	operation=${config_values[1]}
	control=${config_values[2]}
fi
[[ $real = /* && -x $real && $control = /* ]] || { echo "invalid cancellation blocker values" >&2; exit 2; }

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
