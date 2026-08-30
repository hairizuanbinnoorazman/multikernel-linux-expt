#!/usr/bin/env bash
set -euo pipefail

# Cloud-only provisioning for the persistent ext4 experiment. This script never
# formats a guest block device and intentionally attaches only child A until the
# device-granularity audit passes.

PROJECT=${MK_PROJECT:-${PROJECT:-}}
ZONE=${MK_ZONE:-${ZONE:-asia-southeast1-b}}
INSTANCE=${MK_VM:-${INSTANCE:-mklinux-lab}}
MACHINE_TYPE=${MACHINE_TYPE:-n2-standard-16}
BOOT_DISK=${BOOT_DISK:-mklinux-lab}
SOURCE_SNAPSHOT=${SOURCE_SNAPSHOT:-mklinux-lab-pre-daxfs-20260828-2030}
CHILD_A_DISK=${CHILD_A_DISK:-child-a-root}
CHILD_B_DISK=${CHILD_B_DISK:-child-b-root}
CHILD_A_DEVICE_NAME=${CHILD_A_DEVICE_NAME:-mk-child-a-root}

if [[ -z "$PROJECT" ]]; then
  printf 'Set MK_PROJECT or PROJECT.\n' >&2
  exit 2
fi

disk_exists() {
  gcloud compute disks describe "$1" \
    --project="$PROJECT" --zone="$ZONE" >/dev/null 2>&1
}

if ! disk_exists "$BOOT_DISK"; then
  gcloud compute disks create "$BOOT_DISK" \
    --project="$PROJECT" --zone="$ZONE" --type=pd-balanced --size=100GB \
    --source-snapshot="$SOURCE_SNAPSHOT"
fi

if ! gcloud compute instances describe "$INSTANCE" \
  --project="$PROJECT" --zone="$ZONE" >/dev/null 2>&1; then
  gcloud compute instances create "$INSTANCE" \
    --project="$PROJECT" --zone="$ZONE" --machine-type="$MACHINE_TYPE" \
    --disk="name=$BOOT_DISK,boot=yes,auto-delete=yes" \
    --no-shielded-secure-boot --shielded-vtpm \
    --shielded-integrity-monitoring --metadata=serial-port-enable=true \
    --labels=purpose=multikernel-ext4-experiment
fi

for child_disk in "$CHILD_A_DISK" "$CHILD_B_DISK"; do
  if ! disk_exists "$child_disk"; then
    gcloud compute disks create "$child_disk" \
      --project="$PROJECT" --zone="$ZONE" --type=pd-balanced --size=10GB \
      --labels=purpose=multikernel-ext4-child-root
  fi
done

attached_names=$(gcloud compute instances describe "$INSTANCE" \
  --project="$PROJECT" --zone="$ZONE" \
  --format='value(disks.deviceName)')
if ! grep -qw -- "$CHILD_A_DEVICE_NAME" <<<"$attached_names"; then
  gcloud compute instances attach-disk "$INSTANCE" \
    --project="$PROJECT" --zone="$ZONE" --disk="$CHILD_A_DISK" \
    --device-name="$CHILD_A_DEVICE_NAME"
fi

gcloud compute instances describe "$INSTANCE" \
  --project="$PROJECT" --zone="$ZONE" \
  --format='table(name,status,machineType.basename(),disks[].deviceName,disks[].autoDelete)'
gcloud compute disks list --project="$PROJECT" \
  --filter="zone:($ZONE) AND name:($BOOT_DISK $CHILD_A_DISK $CHILD_B_DISK)" \
  --format='table(name,zone.basename(),sizeGb,type.basename(),status,users.basename())'
