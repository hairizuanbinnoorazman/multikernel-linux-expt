#!/usr/bin/env bash
set -euo pipefail

project=${MK_PROJECT:-}

if [[ -z "$project" ]]; then
	if ! command -v gcloud >/dev/null 2>&1; then
		echo 'MK_PROJECT is unset and gcloud is not installed' >&2
		exit 1
	fi
	project=$(gcloud config get-value project 2>/dev/null || true)
fi

if [[ -z "$project" || "$project" == "(unset)" ]]; then
	echo 'set MK_PROJECT or select a project with gcloud config set project PROJECT_ID' >&2
	exit 1
fi

printf '%s\n' "$project"
