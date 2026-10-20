# Qualified host containerd checkpoint policy

The G7 disposable host uses upstream containerd **2.2.10**, with the repository's
CRI fragment setting both `enable_criu` and
`enable_experimental_restore_via_create` to false. Default runc and the opt-in
Multikernel handler coexist. This prevents CRI checkpoint/restore use on that
host; application checkpoint/restore is not a supported Multikernel feature.
The [upstream security advisory](https://github.com/containerd/containerd/security/advisories/GHSA-p7v4-vr35-mj6f)
explains the destination security-context bypass. The
[2.2.10 release](https://github.com/containerd/containerd/releases/tag/v2.2.10)
also includes later upstream fixes. This is a scoped mitigation and update,
not a claim that every host dependency finding is resolved.

The exact artifact and verification commands are retained in the
[qualification proof](../../evidence/runtime-20261012/g7-host-containerd-security/README.md).
Artifact SHA-256 is
`5adda535e65393c4c24f88771ccc4604906d0b0128d448bf74b5336153753b1b`.
`gh attestation verify` was constrained to repository `containerd/containerd`,
workflow `.github/workflows/release.yml`, tag `refs/tags/v2.2.10`, and source commit
`991ca6a8ca58b47346a42093210099e090bc6c1f`. Preserve the attestation and verification
receipt together with any transferred artifact.

The upstream binaries are installed under
`/opt/mkruntime/containerd/v2.2.10/bin`. A root-owned systemd drop-in,
`/etc/systemd/system/containerd.service.d/30-multikernel-security.conf`, selects
that daemon and prepends its directory to the service's PATH for matching runc
shim lookup. The original distribution package binaries stay installed. Its
`/usr/bin/ctr` client remains available; client and running server versions can
differ. Package updates do not advance this explicit service override. Future
host updates must qualify and change the selected release, then verify the
actual running executable, CRI flags and both runtime handlers again.

The main configuration is retained, including the existing fragment import and
runc default. Only the runtime fragment differs in the new immutable support
generation. Do not replace the host's main configuration with a partial example.
Older daemons can ignore unknown configuration keys. A successful parse alone
is insufficient: the live CRI verbose Status response must expose both false
booleans, and the synthetic CheckpointContainer request must return the explicit
configuration-disabled refusal. The retained probe emits only those booleans,
two handler types, the default handler and a refusal assertion. Full verbose
Status/config dumps can contain registry credentials and must stay private.

The qualification first starts an isolated daemon with private root/state/socket
paths and no host registry configuration. It then admits an idle host, stops
Docker/containerd, takes a root-private cold copy of `/var/lib/containerd`,
activates the candidate and changed support generation, exercises actual package
rollback, and reactivates the candidate. It never restores primary runtime state.
All five runtime state hashes must remain identical through this startup
exercise. The root-private rollback copy and previous support generation remain
on the disposable host. A later rollback after workloads have changed state
requires a fresh compatibility/recovery assessment; this startup exercise does
not authorize replaying old state over new records.

Run the source contract against the selected binary:

```bash
MK_CONTAINERD=/opt/mkruntime/containerd/v2.2.10/bin/containerd \
  python3 scripts/test-runtime-containerd-config.py
```

The contract uses config version 3 on 2.2 and version 4 on 2.3, checks the patch
floor and verifies effective flags and handler coexistence. Other release lines
need their own configuration/security qualification before extending this check.
