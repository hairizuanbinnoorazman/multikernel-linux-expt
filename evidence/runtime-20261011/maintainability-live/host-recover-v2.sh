#!/usr/bin/env bash
set -euo pipefail
cd /var/tmp/mk-maint-src
export PATH=/opt/go/bin:$PATH TMPDIR=/var/tmp GOTMPDIR=/var/tmp
[[ $(uname -r) = 7.0.0-mk2-gce-lab ]]
sudo systemctl start sys-fs-multikernel.mount
mountpoint -q /sys/fs/multikernel
mountpoint -q /srv/multikernel-storage
[[ -z $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit) ]]
[[ $(sudo blkid -s UUID -o value /dev/disk/by-id/google-mk-maint-storage) = 00000000-0000-0000-0000-00000000000e ]]
sudo systemctl stop docker containerd mknetd mkruntimed || true
sudo install -d -m 0700 /var/tmp/mk-maint-failed-state
for state in mkruntime mkruntimed mknetd; do
 if sudo test -d /var/lib/$state; then sudo mv /var/lib/$state /var/tmp/mk-maint-failed-state/$state; fi
done
sudo mv /srv/multikernel-storage/runtime /srv/multikernel-storage/failed-matrix-roots
sudo install -d -m0700 /srv/multikernel-storage/runtime
sudo scripts/manage-runtime-deployment.py activate 78a467a46de8cafc794167140e8f9a323e7245233b83069891418d7dec9247a2
sudo systemctl daemon-reload
sudo modprobe mk_transport nbd
sudo systemctl reset-failed mkruntimed mknetd
sudo systemctl enable --now mkruntimed mknetd containerd docker
for service in mkruntimed mknetd containerd docker; do
 test "$(systemctl is-active "$service")" = active
 test "$(systemctl show "$service" -p NRestarts --value)" = 0
done
sudo /usr/local/bin/mk-host-check --kerf=/opt/mkruntime/kerf-venv/bin/kerf --probe-pool-cpus=8,10,12,14,9,11,13,15 --probe-pool-memory=16GB
sudo ctr run --rm --runtime io.containerd.multikernel.v2 docker.io/library/busybox:1.36 mk-maint-bootstrap-v3 /bin/echo MAINT_STABLE_BOOTSTRAP_PASS
sudo systemctl restart mkruntimed
cat /proc/sys/kernel/random/boot_id
printf 'MAINT_HOST_RECOVERY_PASS\n'
