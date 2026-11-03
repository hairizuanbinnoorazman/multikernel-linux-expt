# Multikernel Linux experiments and container runtime

An experimental containerd/Docker runtime that runs workloads on direct
Multikernel Linux child kernels. Kerf manages CPU and memory allocation; the
runtime prepares private writable roots and primary-mediated networking.

The goal is to let host Docker select a child-kernel runtime, with Docker builds
inside an approved child using that child's own engine and storage. This repo
also contains reproducible Google Compute Engine experiments and their evidence.
Kubernetes integration is planned after the standalone runtime gates.

## Quick start with Docker

Start with the [operator setup guide](docs/guides/runtime-operator-quickstart.md)
to prepare a qualified Multikernel host, install Kerf and runtime services, and
validate kernel/bootstrap artifacts. Then follow the
[Docker registration instructions](deploy/README.md#docker-registration) to add
the opt-in handler while keeping the existing default runtime.

On that prepared host, compare the primary boot ID with a child workload:

```bash
cat /proc/sys/kernel/random/boot_id
sudo docker pull docker.io/library/busybox:1.36
sudo docker run --rm \
  --runtime io.containerd.multikernel.v2 \
  --network none \
  docker.io/library/busybox:1.36 \
  /bin/sh -c 'uname -r; cat /proc/sys/kernel/random/boot_id; echo hello-from-child'
```

The container should print its child kernel release, a boot ID different from
the primary, and `hello-from-child`. Select the runtime explicitly;
`--network none` lets the Multikernel runtime own the child link.

The [full usage guide](docs/guides/using-the-runtime.md) covers Docker and `ctr`
lifecycles, interactive shells, inputs and networking. Follow the
[cleanup and recovery guide](docs/guides/runtime-recovery-and-cleanup.md) when a
workload fails or resources remain after removal.

## Docker builds inside a child

The BusyBox example above runs an ordinary workload. Inner `docker build` and
`docker run` require a dedicated [approved Docker guest profile](docs/runtime/contracts/docker-guest-profile.md)
with a guest-local rootless engine and writable storage. Keep the primary Docker
socket outside the child. See the [guest Docker design and examples](docs/runtime/research/docker-in-docker-guests.md)
for bootstrap details, qualified cases and remaining limits.

## Status and deployment boundary

The containerd/Docker MVP has passed gates G0–G6. Security/reliability (G7),
operability (G8) and the standalone preview remain in progress. Use this
experimental runtime on trusted, single-tenant lab hosts; see
[project status](docs/runtime/status.md) and the [gate checklist](docs/runtime/plans/README.md#master-gate-checklist)
for demonstrated behavior and open work.

**Nested virtualization is unavailable in the target cloud instances.** The
runtime uses direct child kernels. Treat the primary and children as one trust
domain after child-kernel compromise. Read the
[architecture and containment boundary](docs/runtime/architecture.md#deployment-and-containment-constraint)
before deploying or proposing isolation changes.

## Local development

From a checkout with Go 1.26.9 or newer installed:

```bash
make runtime-build
make runtime-test
```

The [developer workflow](docs/guides/developer-workflow.md) covers dependencies,
race tests, repository checks and Git LFS setup. Host provisioning and live
cloud tests are separate workflows in the [GCE laboratory runbook](docs/guides/gce-lab-runbook.md).

## Documentation

| Looking for | Start here |
| --- | --- |
| Installation and first workload | [Operator quickstart](docs/guides/runtime-operator-quickstart.md) |
| Docker and containerd examples | [Runtime usage](docs/guides/using-the-runtime.md) |
| System design and ownership | [Architecture](docs/runtime/architecture.md) |
| Current results and open gates | [Status](docs/runtime/status.md) · [Roadmap](docs/runtime/plans/README.md) |
| Experiment reports and proof | [Experiment index](docs/experiments/README.md) · [Evidence runbook](docs/guides/runtime-evidence-runbook.md) |
| Detailed checkpoint history | [Archived runtime notes](docs/runtime/history/readme-checkpoints-2026-11-03.md) |
| All guides and references | [Documentation map](docs/README.md) |
