#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
required_paths=(
  docs/architecture/TARGET.md
  docs/decisions/0001-build-a-new-runtime.md
  docs/plans/README.md
  docs/plans/10-final-build-and-release.md
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
  done < <(sed -nE 's/.*\]\(([^ )]+)( "[^"]*")?\).*/\1/p' "$repo_root/$markdown_file")
done < <(cd "$repo_root" && rg --files -g '*.md' -g '!evidence/**' | sort)

if (( status != 0 )); then
  exit "$status"
fi

printf 'documentation structure and local links: PASS\n'
