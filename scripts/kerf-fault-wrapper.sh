#!/usr/bin/env bash
set -euo pipefail

# Disposable-host test adapter. Install the real pinned Kerf executable at the
# fixed path below and point the strict host config at this root-owned wrapper.
real=/opt/mkruntime/bin/kerf-real
control=/run/mkruntime-kerf-fault

if [[ -f $control ]]; then
	read -r operation <"$control" || operation=
	if [[ ${1:-} == "$operation" ]]; then
		rm -f "$control"
		printf 'injected pre-commit Kerf %s failure\n' "$operation" >&2
		exit 42
	fi
fi
exec "$real" "$@"
