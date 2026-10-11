#!/usr/bin/env bash
set -euxo pipefail

source_root=/var/tmp/mksrc-2ee14c4
expected=/usr/local/lib/multikernel/releases/0.1.0-dev-2ee14c49a6dd674629ba18a0ca618c1870311ca1
stress_evidence=/var/tmp/g5-network-stress-post-reset.log
stress_sha=c5baafb654e660fc784ac18d4bc5b42563e7837447c30d4202bc213ac8264287

test "$(readlink -f /usr/local/lib/multikernel/current)" = "$expected"
test "$(sha256sum "$stress_evidence" | awk '{print $1}')" = "$stress_sha"
grep -Fxq G5_NETWORK_STRESS_LIVE_PASS "$stress_evidence"
grep -Fq '"exit_status": 0' "$stress_evidence"
echo "OBSERVATION_BEGIN key=prior-stress-evidence"
echo "sha256=$stress_sha terminal=G5_NETWORK_STRESS_LIVE_PASS exit_status=0"
echo "OBSERVATION_END key=prior-stress-evidence"
echo 'EVIDENCE_ASSERTION id=mtu-frame-fault-and-load-matrix passed=true'

clean_boundary() {
	local output=
	sudo systemctl restart mkruntimed
	for _ in $(seq 1 240); do
		if sudo test -S /run/mkruntimed.sock &&
			output=$(MK_EVIDENCE_XTRACE=0 bash "$source_root/scripts/audit-runtime-final-resources-live.sh" "$source_root" 2>&1); then
			printf '%s\n' "$output"
			return 0
		fi
		sleep .25
	done
	printf '%s\n' "$output" >&2
	return 1
}

clean_boundary
bash "$source_root/scripts/test-runtime-network-restart-live.sh" "$source_root"
echo 'EVIDENCE_ASSERTION id=transport-child-service-daemon-and-primary-restarts passed=true'
echo 'EVIDENCE_ASSERTION id=primary-health-and-controller-ownership passed=true'
clean_boundary
echo 'EVIDENCE_ASSERTION id=network-resource-cleanup passed=true'
echo G5_FINAL_EXACT_RELEASE_PASS
