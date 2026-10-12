# G7 security boundary assessment: SEC-01, SEC-02, SEC-11

Assessment started: 2026-10-12. Executor and source reviewer: Codex.
Independent reviewer: not yet assigned. This is an engineering assessment,
not an independent penetration test or full G7 qualification.

## Conclusion and confidence statement

The pinned architecture supports approved child kernels and trusted workloads
on a single-tenant node. It does not establish containment after arbitrary
child-kernel execution. No evidence presently supports a claim that this
runtime is harder to escape than gVisor.

Confidence must be attached to an individual claim and its evidence. Passing
unit tests is evidence about those behaviors, not a percentage probability
that the repository is secure. The review below records both protective
behavior and the reasons stronger security claims are blocked.

| Claim | Assessment | Evidence basis / limit |
| --- | --- | --- |
| The audited boot path enters the child at kernel privilege with child-controlled first-level page tables | High confidence in the source conclusion | Exact pinned assembly and spawn source, with hashes also checked on a restored qualified host |
| Assigned E820 regions and initial mappings independently contain a malicious child kernel | Unsupported | These are software ownership/discovery mechanisms; the direct boot path does not install a separate child-specific second-level boundary |
| The restored host matches the previously qualified kernel binary | Verified artifact identity | `vmlinux` SHA-256 matches the retained October 11 manifest; this is not a fresh full host qualification |
| OCI validation and selected execution constraints behave as tested | Verified portable behavior | Fresh agent race-suite results; many lifecycle tests use no-chroot fixtures and cannot establish child confinement |
| Tested workload profiles cannot read the bootstrap credential through kernel/agent command lines | Verified after remediation in four profiles; baseline failed | Private one-shot initramfs credential handoff; no command-line credential markers observed. Other routes and mediated production qualification remain separate |
| Root workloads are universally confined to the OCI filesystem | Unsupported as a general security claim | The tested bootstrap canary route is denied after privilege remediation; child procfs is shared and comprehensive confinement is not established |
| This runtime provides stronger escape resistance than gVisor | Unproven | No matched, pinned comparison has been performed; the post-child-kernel-compromise boundary is missing |

## Evidence identity and reproducibility

The [evidence bundle](../../../evidence/runtime-20261012/g7-security-boundary/README.md)
records the exact dirty workspace, reviewed file hashes, compiled agent hashes,
command outcomes, and limitations. The kernel pin is
`3bdd35b64413da0b4e089ce931bfc2e8b031cbf7`, Kerf pin is
`8b72b3e9b266f8d32e707e2c1743ad7afc50b1ec`, and the restored host's
`vmlinux` digest is
`5cdf26d0d34bfc8ab3d298d99f8a1e189aa6e2dba9be1f4cb2968078548a3c10`.

The [source-audit helper](../../../scripts/audit-runtime-isolation-source.py)
verifies nine immutable upstream files and emits review anchors. The original
seven G1 file hashes reproduced exactly, including the host's copies. This
binds conclusions to reviewed source rather than a mutable upstream branch.
The transport module's local build-safety patch is separate from the direct
boot/memory audit and must be recorded for a live transport fixture.

The subsequent [live authority evidence](../../../evidence/runtime-20261012/g7-authority-live/README.md)
records three completed profiles, earlier failed attempts, full transport-module
build configuration, the build diff, and cloud cleanup. It supplements the
historical bundle, whose pending-upload status remains unchanged.

## SEC-01: attacker tiers and source re-audit

The attacker tiers are process execution, child root with the actually granted
capabilities, and arbitrary child-kernel execution. Primary administrative
compromise is outside containment guarantees. Root inside an OCI chroot is not
automatically arbitrary kernel execution, but granted kernel-administration
capabilities and accessible kernel interfaces can shorten that transition.

The assembly reloads `__KERNEL_CS`, writes CR3, and jumps to the child entry.
The spawn code builds ordinary identity mappings for allocated memory; E820
lists assigned regions as RAM. Both guide a cooperative Linux instance.
Neither prevents an attacker already executing in that kernel from replacing
its first-level mappings. The outer GCE hypervisor bounds the VM, which contains
both primary and child; it does not supply separate EPT domains for them.

CPU parking, membership checks, receiver ring-index masking, packet bounds, and
child-specific shutdown handlers are useful correctness and fault-containment
controls. They do not mediate every privileged instruction or protect shared
memory against arbitrary ring-0 writes. The DMA heap explicitly exposes shared
physical buffers; device mediation avoids assigning the primary controller but
does not create CPU memory isolation.

Disposition: source assessment completed with an architectural limitation.
Retain the existing trusted-child contract. A hostile-child security claim
requires an independently enforced memory/instruction boundary and a new
review; it cannot be achieved by stronger RPC authentication alone.

## SEC-02: workload authority review

The **baseline** source review established these paths to measure (the frozen
source and binary identity is in the initial evidence bundle):

- `guest/mk-agent-init` mounts child procfs/devtmpfs/sysfs and binds them into
  the OCI root. The proc bind has `nosuid,noexec,nodev`, which does not make
  ordinary proc files confidential or introduce a PID namespace. Sysfs is
  remounted read-only, but privileged workloads may have other kernel interfaces.
- `mk.token` is passed in the child kernel command line, then supplied to
  `mk-agent` through `--token-hex`. The private Unix socket directory is outside
  the workload root and mode 0700; these command-line paths can expose the
  credential independently of direct socket access.
- `Manager.StartWithSize` applies chroot and UID/GID/groups and supplies the
  requested environment. It does not request new child PID/mount/user namespaces.
- When OCI capabilities are omitted, the manager does not require a default
  reduced set. Explicit sets use bounding-set drops, capset, and ambient handling.
  `noNewPrivileges` is applied only when requested. Confirm the actual result
  after exec rather than inferring it solely from UID.
- Agent projection rejects namespaces/resources/seccomp. The primary converter
  handles selected standard OCI inputs for a dedicated child; rejection or
  translation is not proof of an independently enforced hostile-child boundary.
- The restored kernel enables modules, `/dev/mem`, and x86 I/O privilege support;
  enforced module signing is disabled in its configuration. Configuration alone
  does not prove that a workload can use these facilities: permissions,
  capabilities, lockdown, and device visibility must also be measured.

The diagnostic harness boots separate children for non-root, root with explicit
empty capabilities plus `noNewPrivileges`, and root with omitted capabilities.
It collects identity, capabilities, mounts, device permissions, descriptors,
credential-visibility booleans, and exact synthetic child-bootstrap canary
access. It never prints tokens or scans arbitrary primary/sibling memory.
It uses the current production agent/bootstrap with a diagnostic initramfs;
mediated-root and Docker-specific policy need separate qualification.

### Live authority results

All three diagnostic workloads exited successfully on the pinned restored host.
Their security observations include failures; successful collection does not
mean that confidentiality or confinement passed.

| Observation | Non-root UID/GID 1234 | Root, empty capabilities and NNP | Root, capabilities omitted |
| --- | --- | --- | --- |
| Effective capabilities | `0` | `0` | `000001ffffffffff` (all 41 measured capability bits) |
| Bounding capabilities | `000001ffffffffff` | `0` | `000001ffffffffff` |
| NoNewPrivs / Seccomp | `0 / 0` | `1 / 0` | `0 / 0` |
| Kernel command-line credential visible | Yes | Yes | Yes |
| Agent argument credential visible | Yes | Yes | Yes |
| Exact child-bootstrap canary read/write | No / No | No / No | Yes / Yes |
| Bootstrap agent socket via `/proc/1/root` visible | No | No | Yes |

The omitted-capability root profile includes `CAP_SYS_MODULE`, `CAP_SYS_RAWIO`,
`CAP_SYS_ADMIN`, and `CAP_SYS_PTRACE`. Both root profiles reported filesystem
read/write permission for `/dev/mem` and `/dev/port`; only the default root
profile had the relevant effective capabilities. Permission predicates do not
establish that an open, ioctl, memory access, module load, or sysctl mutation
would succeed. None of those operations was attempted. `/dev/nbd0` and
`/dev/kmem` were absent in this diagnostic fixture. The retained mount and
descriptor inventories show shared child procfs and ordinary workload pipes;
they do not establish descriptor safety across every production lifecycle.

The exact canary access demonstrates reaching the **child bootstrap filesystem
outside the OCI chroot**, not reaching the primary host. All profiles could
recognize credential-bearing command lines without emitting credential values.
No stolen-credential RPC or primary/sibling memory access was attempted.

Earlier attempts exposed a bootstrap ordering bug: mounting devtmpfs hid the
previously created `/dev/shm` and `/dev/mqueue` directories. The production
bootstrap now recreates them after mounting `/dev`, and three successful boots
exercise that correction. Fixture corrections set private config permissions,
wait for relay readiness, and account for the API omitting zero exit codes.
Failed attempts and their exact transferred sources are retained separately.

### Remediation and verification rerun

The [remediation bundle](../../../evidence/runtime-20261012/g7-authority-remediation/README.md)
records four successful workload profiles and two fail-closed requests on a
new disposable host. It uses the production Kerf loader to append a private,
root-owned credential archive to the approved image through a sealed memory
file. The agent consumes/deletes the file and disables dumpability; neither
kernel nor agent command lines carry the credential. Mediated bootstrap moves
this credential into private tmpfs before switching roots, keeping it off the
storage disk. That mediated path still requires live production qualification.

Omitted root capabilities now default to an empty set. Production execution
always enables no-new-privileges. Only the documented ordinary capability set
is allowed; root bounding/permitted/effective sets must match to avoid UID 0
exec silently restoring wider grants. Kernel-administration, raw-I/O, ptrace,
and other excluded capabilities are rejected. Workload devices are an explicit
ordinary set rather than the full bootstrap devtmpfs. See the
[authority contract](../contracts/workload-authority.md) for compatibility changes.

| Rerun observation | Non-root | Root, empty capabilities | Root, omitted capabilities | Root, 14 allowed capabilities |
| --- | --- | --- | --- | --- |
| Effective capabilities | Zero | Zero | Zero | `00000000a80425fb` |
| NoNewPrivs | 1 | 1 | 1 | 1 |
| Kernel/agent command-line token visible | No / No | No / No | No / No | No / No |
| Child-bootstrap canary read/write | No / No | No / No | No / No | No / No |
| `/dev/mem`, `/dev/port`, `/dev/kmsg`, `/dev/nbd0` exported | No | No | No | No |

Requests for `CAP_SYS_PTRACE` and for `noNewPrivileges: false` were rejected
before workload start. Portable tests additionally exercise every excluded
capability, unsafe token files, unequal root capability contracts, and independent
cpio decoding. The full Go race suite and OCI validator passed on the changed
sources. The initial live attempt was correctly rejected because the snapshot
kernel file was group/other-writable; a protected byte-identical copy passed.
Both attempts and exact uploaded/built inputs are retained. All six rerun
children were removed; CPUs/pool/helpers and cloud resources returned to baseline.

Disposition: the measured credential exposures and canary route are remediated
and verified in this diagnostic scope. SEC-02 remains open for mediated-root and
Docker qualification, remaining shared-procfs/root-signal authority review, and
independent review. No primary/sibling escape or post-kernel-compromise
containment claim follows from these passes.

## SEC-11: containment after child-kernel compromise

The source re-audit is sufficient to identify the missing architectural barrier.
A live primary-memory overwrite is not needed to establish that E820 and
first-level page tables are controlled by the child kernel itself.

No arbitrary-kernel payload, primary/sibling memory read, module-loading test,
APIC/MSR abuse, or destructive host probe is part of this assessment. The
child-bootstrap filesystem canary is a SEC-02 workload-authority fixture; access
to it must never be described as a demonstrated escape to the primary host.

Disposition: post-compromise source assessment completed. Primary-memory attack
reachability is not live-demonstrated. Security confidence rests on the explicit
trusted-child assumption, not on a claim that a compromised child remains contained.

## Findings and next decisions

| Finding | Priority and disposition | Required follow-up |
| --- | --- | --- |
| G7-BOUNDARY-01: no child-specific hardware boundary after child-kernel execution | Architectural blocker for hostile-child claims; existing supported scope excludes this attacker | Keep trusted-single-tenant scope or design an independently enforced boundary and requalify |
| G7-AUTHORITY-01: baseline command-line credential exposure | Remediated and verified in four diagnostic profiles | Qualify mediated production boot and Docker policy; review other disclosure routes |
| G7-AUTHORITY-02: baseline default-root bootstrap access and broad authority | Default root now has zero capabilities; tested canary denied for default, empty, and allowed capability profiles | Complete shared-procfs/signal and privileged-interface review; retain trusted-child scope |
| G7-EVIDENCE-01: full G7 and matched gVisor comparison remain incomplete | Release-claim limitation | Execute remaining G7 checks and SEC-13 before making comparative or broad security claims |

The defensible current statement is: "Pinned-source review identifies no
independently enforced hostile-child boundary. Credential and privilege fixes
pass six bounded live verification cases, including four workload profiles and
two rejected requests. Full G7, new production compatibility qualification, and
stronger-than-gVisor escape resistance remain unestablished."
