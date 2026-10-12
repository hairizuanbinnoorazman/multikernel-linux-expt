#!/usr/bin/env bash
set -euo pipefail
cd /var/tmp/mk-maint-src
export PATH=/opt/go/bin:$PATH TMPDIR=/var/tmp GOTMPDIR=/var/tmp
sudo modprobe mk_transport nbd
sudo systemctl reset-failed
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
