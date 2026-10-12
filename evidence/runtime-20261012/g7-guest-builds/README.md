# G7 guest-local container image builds

**Scoped prototype verification passed. Full G7 remains open.** An approved
builder guest builds a multi-stage Dockerfile using `mk-build . image.tar`,
without user privileged/capability/security flags or access to the primary
Docker daemon. This is not yet a default Docker/containerd runtime feature.

The client runs as guest UID 0 with zero effective/bounding capabilities and
NNP=1. Trusted bootstrap creates a RootlessKit user namespace before chroot,
maps namespace root to guest UID/GID 1000, and enables NNP before starting
BuildKit. It provides a local Unix socket, a native snapshotter, a mount point
for the builder root, and read-only guest cgroup discovery. The production agent,
private credential handoff, capability policy and restricted devices are unchanged.

| Check | Result |
| --- | --- |
| Unchanged rootless image launched as a restricted workload | Rejected namespace startup; observed compatibility blocker |
| Automatically prepared builder: `COPY`, `RUN`, multi-stage `COPY --from` | Passed |
| OCI image export and independently decoded generated file | Passed |
| Repeated build | Passed, cache hits observed |
| Intentionally failing `RUN` | Build rejected |
| Client privileges | UID 0, effective/bounding caps zero, NNP=1 |
| Builder privileges | User namespace mapped to guest UID/GID 1000, NNP=1 |
| Command-line credentials and exact bootstrap canary route | Not exposed / access denied in tested profiles |
| Child/pool/helper and disposable cloud cleanup | Verified |

`host/final/` records the frozen-source rerun. `host/build-probe*` and
`live-tests-attempt-*.log` preserve earlier attempts, including failed bootstrap
ordering, filesystem permissions, mapping-helper metadata, cgroup discovery,
root mount and a dynamic-BusyBox scratch-fixture error. `attempt-history.json`
identifies each exact uploaded archive; none of these failures is rewritten as
a pass. `built-image.oci.tar` contains only the synthetic build result, not the
builder filesystem or credential-bearing initramfs.

The pinned image is
`moby/buildkit@sha256:65eca3c01cd1180c8f64565c4e3c5983af5145818b0e6fa863385d099eb73855`;
the guest reports BuildKit v0.34.0 and RootlessKit v3.2.0. Components match the
[authority remediation](../g7-authority-remediation/README.md). Source/archive
hashes, original image configuration, exported rootfs digest, kernel/module
identities and cleanup ledgers are retained. Original `host/kernel.config`
is the transport-module build configuration, not proof of the full configuration
that originally produced the unchanged kernel binary. Actual user-namespace and
Dockerfile execution provides bounded live feature evidence.

Verify offline:

```sh
python3 scripts/verify-g7-guest-build-evidence.py evidence/runtime-20261012/g7-guest-builds
python3 scripts/check-runtime-evidence.py
```

Limits: initramfs-backed, offline scratch build; no registry/package downloads,
production CNI/mediated-root/Docker requalification, persistent cache, concurrency,
daemon restart or adversarial builder-service testing. The guest-local socket
permits all processes that can access it to submit builds and is intended for
one dedicated workload guest. Approved-image bootstrap introduces trusted
mapping helpers; it requires further security/lifecycle review before release.
No independent reviewer or post-guest-kernel-compromise containment was demonstrated.
Project/operator/boot identifiers are pseudonymized; credentials are redacted.

See the [builder design and remaining work](../../../docs/runtime/research/guest-image-builds.md).
