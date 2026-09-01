# Evidence index

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

Evidence captures historical state and may contain terminal formatting such as
CRLF, backspaces, or trailing padding. Do not normalize raw transcripts merely
to satisfy whitespace tooling.

Cloud identity fields are sanitized before commit. `${MK_PROJECT}` and
`${GCE_SERVICE_ACCOUNT}` preserve the field's meaning without publishing the
operator's project or attached identity. To render a private working copy with
locally configured values, use `envsubst` without overwriting retained evidence:

```bash
export MK_PROJECT=$(gcloud config get-value project)
export GCE_SERVICE_ACCOUNT=$(gcloud compute instances describe "$MK_VM" \
  --zone="$MK_ZONE" --format='value(serviceAccounts[0].email)')
envsubst < evidence/runtime-20260901/g4-g6-gce/manifest.json \
  >/tmp/g4-g6-manifest.local.json
```
