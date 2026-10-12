#!/usr/bin/env bash
set -euo pipefail
cd /var/tmp/mk-maint-src
[[ $(uname -r) = 7.0.0-mk2-gce-lab ]]
[[ -z $(sudo find /sys/fs/multikernel/instances -mindepth 1 -maxdepth 1 -print -quit) ]]
[[ -z $(sudo find /srv/multikernel-storage/runtime -mindepth 1 -print -quit) ]]
[[ -z $(sudo ctr tasks list -q) ]]
[[ -z $(sudo ctr containers list -q) ]]
[[ -z $(sudo ctr -n moby tasks list -q) ]]
[[ -z $(sudo ctr -n moby containers list -q) ]]
[[ -z $(sudo docker ps -aq) ]]
! pgrep -f '^/usr/local/lib/multikernel/.*/containerd-shim-multikernel-v2' >/dev/null
! pgrep -f '^(/usr/local/libexec/multikernel/(mkvsock-nbd|mk-agent-relay)|/opt/mkruntime/bin/mkvsock-relay)' >/dev/null
for service in mkruntimed mknetd containerd docker; do
 [[ $(systemctl is-active "$service") = active ]]
 [[ $(systemctl show "$service" -p NRestarts --value) = 0 ]]
 systemctl show "$service" -p ActiveState -p MainPID -p NRestarts
done
[[ $(systemctl is-enabled apt-daily.timer || true) = masked ]]
[[ $(systemctl is-enabled apt-daily-upgrade.timer || true) = masked ]]
sudo python3 - <<'PY'
import hashlib,json,pathlib,runpy
p=pathlib.Path
manager=runpy.run_path('scripts/manage-runtime-binaries.py')
manifest=manager['verify_installed'](p('/'),'0.1.0-maint-test-49bba37bc4b1d584df6799219be20e26054cf126')
kernel=json.loads(p('/etc/mkruntime/kernels/gce-mk2.json').read_text())
agent=next(c for c in manifest['components'] if c['name']=='mk-agent')
assert agent['sha256']==kernel['agent']['sha256']
for name in ('kernel','initramfs','agent','relay'):
 value=kernel[name];assert hashlib.sha256(p(value['path']).read_bytes()).hexdigest()==value['sha256']
for file,field in [('/var/lib/mkruntimed/rootfs/state.json','records'),('/var/lib/mknetd/state.json','endpoints')]:
 assert not json.loads(p(file).read_text())[field]
print(json.dumps({'installed_revision':manifest['revision'],'components':manifest['components'],'guest_agent_sha256':kernel['agent']['sha256'],'empty_rootfs_and_network_state':True},indent=2))
PY
sha256sum /var/tmp/mk-maintainability-source.tar.gz
cat /proc/sys/kernel/random/boot_id
sudo /opt/mkruntime/kerf-venv/bin/kerf show
printf 'MAINT_FINAL_COMPONENT_AND_INVENTORY_AUDIT_PASS\n'
