# G7 online Docker Python prototype

**Scoped online prototype passed. Full G7 remains open.** A fresh dedicated
multikernel guest runs a rootless Docker Engine and builds/runs a Python app
using ordinary Docker commands, without user privileged/capability/security
flags, a primary Docker socket, proxy settings, or a Docker endpoint export.

| Check | Result |
| --- | --- |
| Fresh daemon image store | Empty before guest pull |
| Docker Hub python:3.13-slim pull | Passed; resolved repository digest retained |
| Requests in unmodified base image | Absent |
| Python build RUN installs from HTTPS PyPI | Passed; five packages downloaded without pip cache |
| App runtime | Python 3.13.16, Requests 2.34.2, verified HTTPS status 200 |
| Repeat Docker build | Cache hits observed |
| Package provenance | Pip report URLs and wheel SHA256 match independently collected version-specific PyPI metadata |
| Outer client | Guest UID 0, zero effective/bounding capabilities, NNP=1 |
| Daemon and nested containers | Root maps to guest UID/GID 1000; NNP=1; nested default seccomp active |
| Tested credential/canary/device routes | Protected; workload TUN absent |
| Cleanup | Guest/helper/pool/network cleanup verified; disposable VM and boot disk deleted; cloud baseline restored |

Installed packages: Requests 2.34.2, charset-normalizer 3.5.2, idna 3.20,
urllib3 2.8.0, and certifi 2026.7.22. `python-base-image.json` records the
Docker Hub resolved digest and platform; `python-built-image.json` records the
resulting image identity and command. `pip-install-report.json` and
`pypi-release-hashes.json` retain installation and independent hash receipts.
The Python image was executed live; it was not exported for an offline layer
content audit. No signature or independent attestation is claimed.

`host/final/` contains the frozen-source fresh-guest rerun. Ten earlier attempts
are retained, including module dependency enumeration, BusyBox ip/nsenter
substitution, private bootstrap TUN permissions, missing hosts file, diagnostic
controller response decoding, and Docker's raw netfilter requirement. The tenth
attempt and final rerun passed. `attempt-history.json` binds every attempt to its
uploaded fixture archive and diagnostic controller digest.

Production agent/kernel/credential-handoff identities match the
[authority remediation](../g7-authority-remediation/README.md). The pinned Docker
rootfs is reused from the [offline Docker prototype](../g7-dind/README.md).
Source hashes, exact fixture archives, the diagnostic controller's build identity,
module receipts, network rules and cleanup ledgers are retained. Large rootfs
and credential-bearing initramfs archives are excluded; cloud/operator/boot
identifiers are pseudonymized.

Verify offline from the repository root:

```sh
python3 scripts/verify-g7-dind-online-evidence.py evidence/runtime-20261012/g7-dind-online
python3 scripts/check-runtime-evidence.py
```

The primary uses a diagnostic static TUN/NAT setup and production authenticated
agent packet RPCs. The fixture permits public HTTPS and selected DNS, rejects
listed private/link-local destinations, and drops primary-local access. This
is not full production CNI or adversarial policy requalification. Storage is
initramfs/VFS, cgroup driver is none, and persistence/restart/concurrency/port
publishing/IPv6/resource enforcement remain unqualified. Trusted bootstrap,
mapping helpers, rootless TUN and socket lifecycle need independent review.
No containment after child-kernel compromise or matched gVisor comparison was
established. See [design](../../../docs/runtime/research/docker-online-python-guest.md)
and [example](../../../examples/guest-python-online/README.md).
