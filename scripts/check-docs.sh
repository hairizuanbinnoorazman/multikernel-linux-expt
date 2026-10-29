#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
required_paths=(
  README.md
  docs/experiments/README.md
  docs/guides/gce-lab-runbook.md
  docs/runtime/architecture.md
  docs/runtime/decisions/0001-build-a-new-runtime.md
  docs/runtime/plans/README.md
  docs/runtime/plans/07-security-and-reliability.md
  docs/runtime/plans/08-operability-and-diagnostics.md
  docs/runtime/plans/09-compatibility-and-release-engineering.md
  docs/runtime/plans/10-performance-and-density.md
  docs/runtime/plans/11-standalone-preview.md
  docs/runtime/plans/12-kubernetes-distribution.md
  runtime/README.md
)

for required_path in "${required_paths[@]}"; do
  if [[ ! -e "$repo_root/$required_path" ]]; then
    printf 'missing required path: %s\n' "$required_path" >&2
    exit 1
  fi
done

status=0
while IFS= read -r markdown_file; do
  markdown_dir=$(dirname "$markdown_file")
  while IFS= read -r target; do
    target=${target#<}
    target=${target%>}
    target=${target%%#*}
    target=${target%%\?*}
    [[ -n "$target" ]] || continue
    case "$target" in
      http://*|https://*|mailto:*|/*) continue ;;
    esac
    if [[ ! -e "$repo_root/$markdown_dir/$target" ]]; then
      printf 'broken local link: %s -> %s\n' "$markdown_file" "$target" >&2
      status=1
    fi
  done < <(
    grep -oE '\]\([^ )]+' "$repo_root/$markdown_file" \
      | sed -E 's/^\]\(//'
  )
done < <(cd "$repo_root" && rg --files -g '*.md' | sort)

if (( status != 0 )); then
  exit "$status"
fi

printf 'documentation structure and local links: PASS\n'
python3 "$repo_root/scripts/check-runtime-schemas.py"
python3 "$repo_root/scripts/check-runtime-evidence.py"
python3 "$repo_root/scripts/test-runtime-oci-validation.py"
python3 "$repo_root/scripts/test-runtime-builder-support.py"
python3 "$repo_root/scripts/test-runtime-builder-isolation.py"
bash "$repo_root/scripts/test-native-server-privileges.sh"
bash "$repo_root/scripts/test-native-process-protection.sh"
bash "$repo_root/scripts/test-native-server-sandbox.sh"
bash "$repo_root/scripts/test-native-server-pressure.sh"
bash "$repo_root/scripts/test-native-server-backpressure.sh"
bash "$repo_root/scripts/test-native-storage-peer.sh"
bash "$repo_root/scripts/test-native-relay-peer.sh"
python3 "$repo_root/scripts/test-native-stream-proxy.py"
python3 "$repo_root/scripts/test-native-stream-backpressure.py"
python3 "$repo_root/scripts/test-kernel-vsock-rx-accounting.py"
python3 "$repo_root/scripts/test-kernel-vsock-release.py"
python3 "$repo_root/scripts/test-runtime-bind-materialization.py"
python3 "$repo_root/scripts/test-runtime-bootstrap-validation.py"
python3 "$repo_root/scripts/test-runtime-docker-root-validation.py"
python3 "$repo_root/scripts/test-runtime-docker-outer.py"
python3 "$repo_root/scripts/test-runtime-docker-image-archive.py"
python3 "$repo_root/scripts/test-runtime-firewall-identity.py"
python3 "$repo_root/scripts/test-runtime-service-policy.py"
python3 "$repo_root/scripts/test-runtime-socket-readiness.py"
python3 "$repo_root/scripts/test-runtime-network-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-network-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-combined-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-pool-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-unified-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-component-build-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-operational-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-workload-phase.py"
python3 "$repo_root/scripts/test-runtime-trial-admission.py"
python3 "$repo_root/scripts/test-runtime-trial-helper.py"
python3 "$repo_root/scripts/test-runtime-trial-stage.py"
python3 "$repo_root/scripts/test-runtime-storage-pressure.py"
python3 "$repo_root/scripts/test-runtime-console-capture.py"
python3 "$repo_root/scripts/test-runtime-primary-docker.py"
python3 "$repo_root/scripts/test-runtime-doctor-checkpoint.py"
python3 "$repo_root/scripts/test-runtime-native-record-fault.py"
python3 "$repo_root/scripts/test-native-client-console.py"
python3 "$repo_root/scripts/test-runtime-docker-concurrency.py"
python3 "$repo_root/scripts/test-runtime-docker-workload-policy.py"
python3 "$repo_root/scripts/test-runtime-rootfs-build.py"
python3 "$repo_root/scripts/test-runtime-storage-build.py"
python3 "$repo_root/scripts/test-runtime-safe-publish.py"
python3 "$repo_root/scripts/test-runtime-storage-mount-validation.py"
python3 "$repo_root/scripts/test-runtime-root-validation.py"
python3 "$repo_root/scripts/test-runtime-image-validation.py"
python3 "$repo_root/scripts/test-runtime-docker-config.py"
python3 "$repo_root/scripts/test-runtime-release-manifest.py"
python3 "$repo_root/scripts/test-manage-runtime-binaries.py"
python3 "$repo_root/scripts/test-manage-runtime-deployment.py"
python3 "$repo_root/scripts/test-gce-resource-ledger.py"
python3 "$repo_root/scripts/test-capture-evidence-command.py"
python3 "$repo_root/scripts/test-runtime-containerd-config.py"
python3 "$repo_root/scripts/test-runtime-dependency-inventory.py"
python3 "$repo_root/scripts/test-kerf-code-tree-trust.py"
python3 "$repo_root/scripts/test-audit-g4-g6-evidence.py"
