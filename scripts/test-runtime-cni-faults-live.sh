#!/usr/bin/env bash
set -euo pipefail

if [[ ${MK_EVIDENCE_XTRACE:-1} = 1 ]]; then
	PS4='+${BASH_SOURCE}:${LINENO}: '
	set -x
fi

source_root=${1:?usage: test-runtime-cni-faults-live.sh SOURCE_ROOT}
cni=${MK_CNI:-/opt/cni/bin/multikernel}
network_name=multikernel
container_id=mk-cni-live
partial_id=mk-cni-partial
ifname=eth0
namespace=mk-cni-live
netns=/run/netns/$namespace
missing_netns=/run/netns/mk-cni-missing
scratch=$(mktemp -d -p /var/tmp mk-cni-faults.XXXXXX)
cache=$scratch/cache
config=$scratch/10-multikernel.conf
last_output=
last_error=
last_status=

observe() {
	local key=$1
	shift
	printf 'OBSERVATION_BEGIN key=%s\n%s\nOBSERVATION_END key=%s\n' "$key" "$*" "$key"
}

invoke_cni() {
	local command=$1 id=$2 namespace_path=$3 label=$4
	local output_file=$scratch/$label.stdout error_file=$scratch/$label.stderr
	set +e
	sudo env CNI_COMMAND="$command" CNI_CONTAINERID="$id" CNI_NETNS="$namespace_path" CNI_IFNAME="$ifname" \
		"$cni" <"$config" >"$output_file" 2>"$error_file"
	last_status=$?
	set -e
	last_output=$(<"$output_file")
	last_error=$(<"$error_file")
	observe "cni-$label" "argv=$cni
CNI_COMMAND=$command CNI_CONTAINERID=$id CNI_NETNS=$namespace_path CNI_IFNAME=$ifname
stdin=$(<"$config")
stdout=$last_output
stderr=$last_error
exit_status=$last_status"
}

cleanup() (
	set +e
	if [[ -f $config ]]; then
		invoke_cni DEL "$container_id" "" trap-del-live >/dev/null 2>&1 || true
		invoke_cni DEL "$partial_id" "" trap-del-partial >/dev/null 2>&1 || true
	fi
	sudo ip netns delete "$namespace" >/dev/null 2>&1 || true
	sudo ip netns delete mk-cni-missing >/dev/null 2>&1 || true
	rm -rf -- "$scratch"
)
trap cleanup EXIT

endpoint_count() {
	sudo python3 -c 'import json; print(len(json.load(open("/var/lib/mknetd/state.json"))["endpoints"]))'
}

endpoint_summary() {
	sudo python3 - <<'PY'
import json,re
d=json.load(open('/var/lib/mknetd/state.json'))
assert d['version']==1 and len(d['endpoints'])==1,d
v=next(iter(d['endpoints'].values()))
assert v['owner']=='cni' and not v.get('managed_namespace',False) and v['state']=='READY',v
assert re.fullmatch(r'[0-9a-f]{32}',v['generation']),v
print(json.dumps({k:v[k] for k in ('container_id','network_name','if_name','netns','owner','generation','address','gateway','mtu','state','rx_packets','tx_packets','rx_drops','tx_drops','errors')},sort_keys=True,separators=(',',':')))
PY
}

inventory() {
	local endpoints caches links routes chains nat_rules namespaces
	endpoints=$(endpoint_count)
	caches=$(sudo find "$cache" -mindepth 1 -maxdepth 1 -type f -name '*.json' 2>/dev/null | wc -l)
	links=$(ip -o link show | awk -F': ' '$2 ~ /^mkv[0-9a-f]+$/ {n++} END {print n+0}')
	routes=$(ip -4 route show | grep -cE '^172\.31\.[0-9]+\.[0-9]+/30 ' || true)
	chains=$(sudo iptables -S | grep -cE '^(-N|-A) MK-' || true)
	nat_rules=$(sudo iptables -t nat -S POSTROUTING | grep -c '172\.31\.' || true)
	namespaces=$(sudo ip netns list | awk '$1=="mk-cni-live" || $1=="mk-cni-missing" {n++} END {print n+0}')
	printf 'endpoints=%s caches=%s links=%s routes=%s chains=%s nat_rules=%s test_namespaces=%s' \
		"$endpoints" "$caches" "$links" "$routes" "$chains" "$nat_rules" "$namespaces"
}

wait_inventory() {
	local expected=$1 observed=
	for _ in $(seq 1 120); do
		observed=$(inventory)
		[[ $observed = "$expected" ]] && { printf '%s' "$observed"; return 0; }
		sleep .25
	done
	printf '%s' "$observed"
	return 1
}

live_detail() {
	printf '%s\n' '--- endpoint-state ---'
	sudo cat /var/lib/mknetd/state.json
	printf '%s\n' '--- cache ---'
	sudo find "$cache" -mindepth 1 -maxdepth 1 -type f -printf '%m %u:%g %s %f\n' -exec cat {} \;
	printf '%s\n' '--- primary-links-routes ---'
	ip -details -o link show | grep -E 'mkv[0-9a-f]+' || true
	ip -4 route show | grep -E '^172\.31\.' || true
	printf '%s\n' '--- namespace ---'
	sudo ip netns exec "$namespace" ip -details -o link show
	sudo ip netns exec "$namespace" ip -4 -o address show
	sudo ip netns exec "$namespace" ip -4 route show
	printf '%s\n' '--- filter-rules ---'
	sudo iptables -S | grep -E 'MK-|mkv' || true
	printf '%s\n' '--- nat-rules ---'
	sudo iptables -t nat -S | grep '172\.31\.' || true
}

clean='endpoints=0 caches=0 links=0 routes=0 chains=0 nat_rules=0 test_namespaces=0'
[[ $(id -u) -ne 0 ]] || { echo 'run as an ordinary sudo-capable user' >&2; exit 1; }
test -x "$cni"
test -d "$source_root/runtime"
test -x "$source_root/scripts/audit-runtime-final-resources-live.sh"
for service in mkruntimed mknetd containerd docker; do [[ $(systemctl is-active "$service") = active ]]; done
sudo ip netns delete "$namespace" >/dev/null 2>&1 || true
sudo ip netns delete mk-cni-missing >/dev/null 2>&1 || true
sudo install -d -m 0700 -o root -g root "$cache"
cat >"$config" <<EOF
{"cniVersion":"1.0.0","name":"$network_name","type":"multikernel","socket":"/run/mknetd.sock","cacheDir":"$cache"}
EOF
chmod 0600 "$config"
observe clean-before "$(wait_inventory "$clean")"
observe provenance "boot_id=$(cat /proc/sys/kernel/random/boot_id)
selector=$(readlink -f /usr/local/lib/multikernel/current)
cni_sha256=$(sha256sum "$cni" | awk '{print $1}')
mknetd_sha256=$(sudo sha256sum /proc/$(systemctl show -p MainPID --value mknetd)/exe | awk '{print $1}')
qualifier_sha256=$(sha256sum "$0" | awk '{print $1}')
backend_test_sha256=$(sha256sum "$source_root/runtime/internal/network/backend_linux_test.go" | awk '{print $1}')
cni_test_sha256=$(sha256sum "$source_root/runtime/cmd/mk-cni/main_test.go" | awk '{print $1}')
config_mode_size_sha256=$(stat -c '%a:%s' "$config"):$(sha256sum "$config" | awk '{print $1}')"

(
	cd "$source_root/runtime"
	go test -race -count=20 -v ./internal/network ./cmd/mk-cni \
		-run 'Test(LinuxBackendRollsBackEveryAddBoundary|PartialAddFailureRollsBackWithoutState|FinalAddStateFailureRollsBackDurableAllocatingRecord|ReconcileRemovesInterruptedAllocatingGeneration|DeleteJournalsTransitionAndReconcileCompletesFailure|RepeatedCheckDeleteAndNameReuse|DeleteRecoversIdentityBoundCacheQuarantine)$'
)
observe exact-source-fault-suite 'count=20 race=true result=pass'

invoke_cni ADD "$partial_id" "$missing_netns" partial-add-missing-netns
[[ $last_status -ne 0 ]]
grep -Fq 'endpoint ADD rolled back' <<<"$last_output"
observe partial-add-rollback "$(wait_inventory "$clean")"

sudo ip netns add "$namespace"
invoke_cni ADD "$container_id" "$netns" generation-one-add
[[ $last_status -eq 0 ]]
python3 - "$last_output" <<'PY'
import json,sys
d=json.loads(sys.argv[1]); assert d['cniVersion']=='1.0.0'; assert d['interfaces'][0]['name']=='eth0'; assert d['interfaces'][0]['sandbox']=='/run/netns/mk-cni-live'; assert d['ips'][0]['address'].endswith('/30'); assert d['ips'][0]['gateway']; assert d['dns']['nameservers']
PY
first=$(endpoint_summary)
observe generation-one-live "endpoint=$first
$(live_detail)"

invoke_cni CHECK "$container_id" "$netns" generation-one-check-one
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]
invoke_cni CHECK "$container_id" "$netns" generation-one-check-two
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]

sudo ip netns delete "$namespace"
invoke_cni DEL "$container_id" "" generation-one-stale-del
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]
invoke_cni DEL "$container_id" "" generation-one-repeated-del
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]
observe generation-one-clean "$(wait_inventory "$clean")"

sudo ip netns add "$namespace"
invoke_cni ADD "$container_id" "$netns" generation-two-add
[[ $last_status -eq 0 ]]
second=$(endpoint_summary)
python3 - "$first" "$second" <<'PY'
import json,sys
a,b=map(json.loads,sys.argv[1:])
assert a['container_id']==b['container_id'] and a['netns']==b['netns']
assert a['address']==b['address']
assert a['generation']!=b['generation']
PY
observe generation-two-reuse "first=$first
second=$second
$(live_detail)"
invoke_cni CHECK "$container_id" "$netns" generation-two-check
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]
invoke_cni DEL "$container_id" "" generation-two-del
[[ $last_status -eq 0 && -z $last_output && -z $last_error ]]
sudo ip netns delete "$namespace"
observe clean-after "$(wait_inventory "$clean")"

audit_output=$(MK_EVIDENCE_XTRACE=0 "$source_root/scripts/audit-runtime-final-resources-live.sh")
grep -Fq G6_FINAL_RESOURCE_RETURN_PASS <<<"$audit_output"
printf '%s\n' "$audit_output"

trap - EXIT
rm -rf -- "$scratch"
echo G5_CNI_FAULTS_LIVE_PASS
