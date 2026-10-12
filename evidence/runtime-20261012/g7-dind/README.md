# G7 Docker-in-Docker guest verification

**Scoped prototype verification passed. Full G7 remains open.** Docker Engine
29.9.0 runs inside the dedicated multikernel guest. Users issue ordinary
`docker build -t my-image .` and `docker run --rm my-image`, without privileged,
capability or security flags, socket forwarding, or a `DOCKER_HOST` export.
An approved diagnostic guest image prepares the rootless daemon automatically;
this profile is not yet integrated into the production Docker/containerd shim.

| Check | Result |
| --- | --- |
| Plain dockerd under restricted root workload | Startup rejected: socket chown operation not permitted |
| Prepared daemon and standard Docker CLI | Passed; local guest-only Unix socket |
| Multi-stage COPY, RUN and COPY --from | Passed |
| Repeat build | Cache hits observed |
| Default docker run of built image | Expected generated output |
| Docker save archive | Config, layer digests, executable and generated file independently verified |
| Deliberately failed RUN | Build rejected |
| Outer client | Guest UID 0, effective/bounding capabilities zero, NNP=1 |
| Rootless daemon | Guest UID/GID 1000; namespace root maps to 1000; NNP=1 |
| Nested container | Namespace-local default capabilities, NNP=1, seccomp filter active |
| Tested credentials, bootstrap canary and restricted devices | No exposure/access through tested routes |
| Cloud and host cleanup | No children/helpers; CPUs restored; disposable VM and disk deleted; baseline unchanged |

The pinned filesystem is
`docker@sha256:54155db4950e2f9af3bb829fcfe627030576fc7ae235018be63175fca7326171`.
Production agent binaries and the kernel match the
[authority remediation](../g7-authority-remediation/README.md). Additional guest
bridge/veth module hashes are recorded in the constructor and cleanup receipts.
The retained kernel configuration describes the transport-module build, not an
independently recovered original full kernel configuration.

`host/final/` contains the frozen-source baseline/prepared rerun. Two earlier
attempts and their exact source archives are retained: the first failed because
the bootstrap shell PATH omitted the Docker binaries; the second passed.
`attempt-history.json`, `source-hashes.json`, `tested-source.tar.gz` and
`fixtures.tar.gz` bind the evidence to source and uploaded fixture identities.
`built-image.docker.tar.gz` contains only the synthetic scratch image with static
BusyBox and `/result`. Large daemon rootfs and credential-bearing initramfs
archives are excluded. Project/operator/boot identifiers are pseudonymized.

Verify from the repository root:

```sh
python3 scripts/verify-g7-dind-evidence.py evidence/runtime-20261012/g7-dind
python3 scripts/check-runtime-evidence.py
```

This is an offline initramfs-backed prototype using VFS storage. Registry pulls,
package downloads, egress, published ports, persistence, restart, concurrency,
resource-limit enforcement and mediated storage remain unqualified. The local
socket permits control of this engine by processes in the dedicated guest, so
the guest represents one trust domain. Trusted namespace/mapping-helper setup,
socket authority and service lifecycle require independent review before release.
No containment after child-kernel compromise or matched superiority over gVisor
was established. See the [design and remaining work](../../../docs/runtime/research/docker-in-docker-guests.md).
