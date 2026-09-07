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
  docs/runtime/plans/10-final-build-and-release.md
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
python3 "$repo_root/scripts/test-runtime-bootstrap-validation.py"
python3 "$repo_root/scripts/test-runtime-rootfs-build.py"
python3 "$repo_root/scripts/test-runtime-storage-build.py"
python3 "$repo_root/scripts/test-runtime-root-validation.py"
python3 "$repo_root/scripts/test-runtime-image-validation.py"
python3 "$repo_root/scripts/test-runtime-docker-config.py"
python3 "$repo_root/scripts/test-runtime-release-manifest.py"
python3 "$repo_root/scripts/test-manage-runtime-binaries.py"
python3 "$repo_root/scripts/test-gce-resource-ledger.py"
python3 "$repo_root/scripts/test-capture-evidence-command.py"
python3 "$repo_root/scripts/test-runtime-containerd-config.py"
