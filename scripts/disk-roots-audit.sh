#!/usr/bin/env bash
set -euo pipefail

# Read-only guest audit. Exit 2 means the disk and host root share the same PCI
# allocation unit and must not be placed in a Multikernel/Kerf device pool.

TARGET_BY_ID=${1:-/dev/disk/by-id/google-mk-child-a-root}
EXPECTED_BYTES=${EXPECTED_BYTES:-10737418240}

for command_name in findmnt lsblk readlink stat udevadm wipefs lspci; do
  command -v "$command_name" >/dev/null || {
    printf 'required command missing: %s\n' "$command_name" >&2
    exit 1
  }
done

if [[ ! -L "$TARGET_BY_ID" ]]; then
  printf 'target is not a persistent by-id symlink: %s\n' "$TARGET_BY_ID" >&2
  exit 1
fi

target_device=$(readlink -f -- "$TARGET_BY_ID")
target_name=${target_device##*/}
target_type=$(lsblk -dnro TYPE -- "$target_device")
target_bytes=$(lsblk -bdnro SIZE -- "$target_device")
root_source=$(findmnt -nro SOURCE /)
root_parent=$(lsblk -dnro PKNAME -- "$root_source")
if [[ -n "$root_parent" ]]; then
  root_device=/dev/$root_parent
else
  root_device=$root_source
fi
root_name=${root_device##*/}

pci_ancestor() {
  local block_name=$1
  local node
  local base
  node=$(readlink -f -- "/sys/class/block/$block_name")
  while [[ "$node" != / ]]; do
    base=${node##*/}
    if [[ "$base" =~ ^[[:xdigit:]]{4}:[[:xdigit:]]{2}:[[:xdigit:]]{2}\.[[:xdigit:]]$ ]] &&
       [[ -e "/sys/bus/pci/devices/$base" ]]; then
      printf '%s\n' "$base"
      return 0
    fi
    node=${node%/*}
    [[ -n "$node" ]] || node=/
  done
  return 1
}

target_pci=$(pci_ancestor "$target_name") || {
  printf 'no PCI ancestor found for %s\n' "$target_device" >&2
  exit 1
}
root_pci=$(pci_ancestor "$root_name") || {
  printf 'no PCI ancestor found for root device %s\n' "$root_device" >&2
  exit 1
}

printf 'kernel=%s\n' "$(uname -r)"
printf 'root_source=%s\n' "$root_source"
printf 'root_device=%s\n' "$root_device"
printf 'root_pci=%s\n' "$root_pci"
printf 'target_by_id=%s\n' "$TARGET_BY_ID"
printf 'target_device=%s\n' "$target_device"
printf 'target_type=%s\n' "$target_type"
printf 'target_bytes=%s\n' "$target_bytes"
printf 'target_pci=%s\n' "$target_pci"
printf 'target_serial=%s\n' "$(udevadm info --query=property --name="$target_device" | sed -n 's/^ID_SERIAL_SHORT=//p')"
printf 'target_sysfs=%s\n' "$(readlink -f -- "/sys/class/block/$target_name")"
printf 'root_sysfs=%s\n' "$(readlink -f -- "/sys/class/block/$root_name")"

printf '%s\n' '--- lsblk ---'
lsblk -e7 -o NAME,PATH,MAJ:MIN,SIZE,TYPE,FSTYPE,LABEL,UUID,MOUNTPOINTS,MODEL,SERIAL,HCTL
printf '%s\n' '--- target signatures (read-only) ---'
wipefs -n -- "$target_device"
printf '%s\n' '--- shared PCI function ---'
lspci -s "$target_pci" -nnk

[[ "$target_type" == disk ]] || {
  printf 'REFUSE: target is not a whole disk.\n' >&2
  exit 1
}
[[ "$target_bytes" == "$EXPECTED_BYTES" ]] || {
  printf 'REFUSE: target size is %s, expected %s bytes.\n' \
    "$target_bytes" "$EXPECTED_BYTES" >&2
  exit 1
}

if [[ "$target_pci" == "$root_pci" ]]; then
  printf 'HARD STOP: target and boot disk share PCI function %s.\n' \
    "$target_pci" >&2
  printf 'Pinned Kerf allocates PCI functions, so this disk cannot be handed off independently.\n' >&2
  exit 2
fi

printf 'PASS: target has a PCI allocation unit distinct from the host root.\n'
