# Evidence index

All runtime manifests are checked by `scripts/check-runtime-evidence.py`.
Historical manifests that predate the strict v1 contract are not rewritten;
their exact machine-readable deficiencies and explanations are recorded in
`runtime-manifest-exceptions.json`. An exception is an explicit nonconformance,
not evidence that the affected run passed a gate.

- [`daxfs-20260828/`](daxfs-20260828/README.md): DAXFS, Docker-derived roots,
  shared mounts, corruption, exhaustion, dual kernels, and cleanup.
- [`ext4-disk-20260830/`](ext4-disk-20260830/README.md): direct-device topology
  gate and cloud cleanup.
- [`ext4-mediated-20260830/`](ext4-mediated-20260830/README.md): transport,
  persistent roots, reset/stop-start, dual kernels, filesystem checks, and
  resource disposition.
- [`runtime-20260831/`](runtime-20260831/README.md): G0-G3 contracts, host
  qualification, recoverable daemon, OCI agent, compatibility failures, and
  final cloud cleanup.
- [`runtime-20260901/`](runtime-20260901/README.md): G4-G6 MVP, robustness,
  full shared `ctr`/Docker feature matrix, and final cloud cleanup.
- [`runtime-20260902/`](runtime-20260902/README.md): live guest stdin,
  detach/reattach, terminal/resize revalidation, and final cloud cleanup.

Evidence captures historical state and may contain terminal formatting such as
CRLF, backspaces, or trailing padding. Do not normalize raw transcripts merely
to satisfy whitespace tooling.

Cloud and operator identity fields are sanitized before commit. Placeholders
such as `${MK_PROJECT}`, `${GCE_SERVICE_ACCOUNT}`, `${HOST_BOOT_ID}`,
`${CONTAINERD_SERVER_UUID}`, and `${REPOSITORY_ROOT}` preserve each field's
meaning without publishing the operator's project, attached identity, machine
identity, or home directory. To render a private working copy with locally
derived values, use `envsubst` without overwriting retained evidence:

```bash
export MK_PROJECT="$(./scripts/detect-gcp-project.sh)"
export GCE_SERVICE_ACCOUNT="$(gcloud compute instances describe "$MK_VM" \
  --zone="$MK_ZONE" --format='value(serviceAccounts[0].email)')"
export HOST_BOOT_ID="$(cat /proc/sys/kernel/random/boot_id)"
export CONTAINERD_SERVER_UUID="$(sudo ctr version | awk '/UUID:/ {print $2; exit}')"
export REPOSITORY_ROOT="$(git rev-parse --show-toplevel)"
envsubst < evidence/runtime-20260901/g4-g6-gce/manifest.json \
  >/tmp/g4-g6-manifest.local.json
```
