#!/usr/bin/env bash
set -euo pipefail
cd /var/tmp/mk-maint-src
export PATH=/opt/go/bin:$PATH
sudo chown -R root:root deploy scripts guest
storage_uuid=$(sudo blkid -s UUID -o value /dev/disk/by-id/google-mk-maint-storage)
work=$(mktemp -d -p /var/tmp mkc.XXXXXX)
trap 'rm -rf "$work"' EXIT
cp deploy/systemd/runtime.env.example "$work/runtime.env"
python3 - "$work/runtime.env" "$storage_uuid" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]);s=p.read_text().replace('/dev/disk/by-id/google-mk-mediated-storage-20260830','/dev/disk/by-id/google-mk-maint-storage').replace('MKRUNTIME_STORAGE_SERIAL=mk-mediated-storage-20260830','MKRUNTIME_STORAGE_SERIAL=mk-maint-storage').replace('MKRUNTIME_STORAGE_BYTES=21474836480','MKRUNTIME_STORAGE_BYTES=32212254720').replace('MKRUNTIME_STORAGE_LABEL=mk-mediated-host','MKRUNTIME_STORAGE_LABEL=mk-maint-host').replace('00000000-0000-0000-0000-000000000005',sys.argv[2]);p.write_text(s)
PY
cat > "$work/config.json" <<'JSON'
{"schema_version":1,"state_directory":"/var/lib/mkruntime","socket_path":"/run/mkruntimed.sock","kerf_executable":"/opt/mkruntime/kerf-venv/bin/kerf","multikernel_sysfs_root":"/sys/fs/multikernel","kernel_manifest_directory":"/etc/mkruntime/kernels","min_primary_cpus":4,"min_primary_memory_bytes":8589934592,"forbidden_apic_ids":[0],"backend_timeout_seconds":180,"max_frame_size_bytes":1048576}
JSON
sudo chown root:root "$work/runtime.env" "$work/config.json"
sudo chmod 0600 "$work/runtime.env" "$work/config.json"
sudo scripts/manage-runtime-deployment.py install "$work/runtime.env" "$work/config.json"
sudo systemctl daemon-reload
sudo systemctl start sys-fs-multikernel.mount
sudo scripts/validate-runtime-bootstrap.py /etc/mkruntime/kernels/gce-mk2.json gce-mk2
sudo /usr/local/bin/mk-host-check --kerf=/opt/mkruntime/kerf-venv/bin/kerf --probe-pool-cpus=8,10,12,14,9,11,13,15 --probe-pool-memory=16GB
sudo systemctl enable --now mkruntimed mknetd
# Adapt the pristine distro-generated containerd config to import the supplied fragment.
if ! sudo test -f /etc/containerd/config.toml; then
 sudo containerd config default | sudo tee /etc/containerd/config.toml >/dev/null
fi
sudo python3 - <<'PY'
import pathlib,re,tomllib
p=pathlib.Path('/etc/containerd/config.toml');s=p.read_text();data=tomllib.loads(s)
imports=data.get('imports',[])
if '/etc/containerd/conf.d/*.toml' not in imports:
 imports.append('/etc/containerd/conf.d/*.toml')
 line='imports = '+repr(imports).replace("'",'"')
 if re.search(r'^imports\s*=',s,re.M):s=re.sub(r'^imports\s*=.*$',line,s,count=1,flags=re.M)
 else:s=line+'\n'+s
p.write_text(s)
PY
sudo containerd config dump >/dev/null
sudo systemctl restart containerd
scripts/merge-runtime-docker-config.py --enable-live-restore /etc/docker/daemon.json "$work/docker.json"
sudo dockerd --validate --config-file "$work/docker.json"
sudo install -m 0644 "$work/docker.json" /etc/docker/daemon.json
sudo systemctl restart docker
for service in mkruntimed mknetd containerd docker; do
 systemctl show "$service" -p ActiveState -p MainPID -p NRestarts
 test "$(systemctl is-active "$service")" = active
done
sudo scripts/manage-runtime-binaries.py inspect
sudo docker info --format '{{json .Runtimes}}'
printf 'MAINT_HOST_CONFIGURE_PASS\n'
