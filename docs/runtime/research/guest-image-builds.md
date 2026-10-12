# Building container images inside a multikernel guest

The experimental builder guest provides a local BuildKit service and a client:

```sh
mk-build . image.tar
```

The context contains a normal `Dockerfile`; output is an OCI image archive.
There is no user-facing privileged flag, capability list, namespace setup, or
primary Docker socket. This is a dedicated, approved builder-image prototype,
not yet a default feature of every image launched through Docker/containerd.

The subsequent [Docker-in-Docker prototype](docker-in-docker-guests.md) runs an
actual guest-local Docker Engine and uses ordinary `docker build`/`docker run`
commands; it extends this builder-only proof with daemon and nested-container
verification under the same outer workload restrictions.

## Design and authority

An ordinary workload cannot simply start the exported rootless BuildKit image:
Linux rejects user-namespace creation from the existing OCI chroot, and the
mandatory no-new-privileges policy prevents setuid mapping helpers from gaining
privileges. A separate Linux kernel provides the facilities, but bootstrap must
make them usable before applying the workload restrictions.

The approved builder bootstrap automatically:

1. Gives the builder filesystem a recursive bind mount, retaining the restricted
   device, procfs, sysfs and runtime mounts. runc needs a mount point for `/`.
2. Exposes the **guest** cgroup2 hierarchy read-only for runc discovery.
3. Runs RootlessKit as guest UID/GID 1000 before chroot. Trusted mapping helpers
   map namespace ID 0 to guest ID 1000 and IDs 1-65536 to guest IDs 100000-165535.
4. Enters the builder filesystem and enables no-new-privileges before starting
   BuildKit. Its administration capabilities are local to that user namespace.
5. Exposes a Unix socket inside this dedicated workload guest. `mk-build` selects
   the local endpoint and native snapshotter defaults require no user flags.

Docker export and newc initramfs do not preserve file-capability xattrs. The
diagnostic builder therefore installs root-owned setuid mapping helpers during
trusted image preparation. They are used before accepting workload requests;
workloads still receive zero default capabilities and mandatory NNP. This is a
new trusted bootstrap component, not permission for arbitrary images to execute
their startup programs with guest-root authority.

The socket is mode 0666 within a single dedicated guest and is not exposed on
TCP or the primary. Any process with access to this guest endpoint can submit
builds; it must not be shared between mutually untrusted users. Native snapshots
avoid requiring `/dev/fuse` or an overlay module in the diagnostic image.

## Bounded verification

The live test exercises a pinned `moby/buildkit:rootless` image with the unchanged
production agent and private Kerf credential handoff. It compares the ordinary
workload path with automatic bootstrap preparation. The assertions cover:

- A multi-stage Dockerfile using `COPY`, `RUN`, and `COPY --from` without network
  pulls, and an OCI archive containing the expected generated file.
- A repeated build with cached steps and rejection of an intentionally failing
  `RUN` command.
- Workload UID 0, zero effective/bounding capabilities, and NNP enabled.
- Builder UID mapping and NNP, restricted devices, no command-line credentials,
  and denial of the exact synthetic child-bootstrap canary route.
- Full child/pool/helper cleanup and deletion of the disposable VM/boot disk.

See the [collected evidence](../../../evidence/runtime-20261012/g7-guest-builds/README.md)
for outcomes, source identities, failed attempts and limitations. The test is
[test-runtime-guest-build-live.sh](../../../scripts/test-runtime-guest-build-live.sh);
the image constructor is
[build-buildkit-probe-initramfs.sh](../../../scripts/build-buildkit-probe-initramfs.sh).
Both are disposable-host diagnostics, with preparation handled by the platform.

## Qualification still required

This fixture uses an initramfs root and offline build context. Registry pulls,
package downloads, CNI integration, mediated storage, persistent cache, resource
limits, concurrent jobs, daemon restart and arbitrary Dockerfiles need further
qualification. The wrapper exports OCI archives; it does not implement the
Docker Engine API, tags, push, or a drop-in `docker build` command.

Production integration needs an explicit approved builder profile, immutable
bootstrap inputs, service readiness/lifecycle ownership, scoped socket access,
and the usual storage/network qualification. The diagnostic changes do not
enable arbitrary privileged startup hooks in the production agent init.

The [G7 boundary assessment](g7-security-boundary-assessment.md) remains in force:
successful builds do not establish containment after guest-kernel compromise or
stronger escape resistance than gVisor.
