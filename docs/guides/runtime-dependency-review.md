# Runtime dependency and imported-code review

G7 dependency review keeps unresolved vulnerabilities and provenance/license gaps
explicit. Inventory collection and successful scans are not security acceptance.
The current runtime and host containerd qualifications remain separate from this
review; see the [G7 checklist](../runtime/plans/g7-security-checklist.md).

## Protect Kerf's imported code

The runtime holds and checks its Kerf entrypoint and interpreter before execution.
Python can still import Kerf from an editable source checkout. That checkout and
the selected virtualenv must be protected from other writers, just as executable
files must be. PYTHONSAFEPATH and disabling user-site imports do not protect an
editable module installed through the virtualenv.

For the installed production checkout:

```bash
sudo python3 -I -S scripts/harden-kerf-installation.py \
  --check /opt/mkruntime/kerf
```

On a root-owned pinned tree, `--apply` removes group/other write bits while
preserving contents, ownership and read/execute bits. It never follows a symlink
to chmod its target. External symlink targets must already have protected owners
and path components, including intermediate aliases. Unexpected owners, hard
links, special files, changed contents and unsafe ancestors are refused. It
protects directories before files and compares the final tree content/owner
digest. An optional `--private-plan PATH` retains original modes and identities
in a new root-only file. A failed operation may leave earlier permission
restrictions in place; it does not restore write access automatically.

Root runs of `install-kerf.sh` use umask022, protect the pinned checkout before
make/pip execution, then check the completed tree. Unprivileged development
installs remain available; production runtime paths require root-owned code.
Keep a separately selected virtualenv protected too:

```bash
sudo python3 -I -S scripts/harden-kerf-installation.py \
  --check /opt/mkruntime/kerf-venv
```

This is an installation permission check, not an attestation of every Python
import, source authenticity or protection against a malicious primary root.
Primary root remains trusted to maintain installed code. A source pin alone
does not prove a clean installed module or protected filesystem permissions.

## Collect a private dependency inventory

Build the standard-library-only metadata inspector with the qualified toolchain
and install it in a protected directory. It reads Go build information and ELF
library dependencies without executing the target binaries.

```bash
GOTOOLCHAIN=go1.26.9 CGO_ENABLED=0 GOCACHE=/tmp/mk-go-cache \
  go build -trimpath -buildvcs=false -o /tmp/mk-dependency-buildinfo \
  tools/mk-dependency-buildinfo.go
sudo install -d -m 0755 /opt/mkruntime/security-tools
sudo install -o root -g root -m 0755 /tmp/mk-dependency-buildinfo \
  /opt/mkruntime/security-tools/buildinfo
sudo install -d -m 0700 /var/lib/mkruntime/dependency-reviews
sudo python3 -I -S scripts/collect-runtime-dependencies.py \
  --buildinfo /opt/mkruntime/security-tools/buildinfo \
  --output /var/lib/mkruntime/dependency-reviews/inventory.json
```

The output must be a new file and is created mode0600. It contains internal
artifact paths and software/version metadata and should remain private. The
collector hashes selected runtime/profile artifacts and support files, records
active daemon identities, installed packages, package copyright metadata,
recursive ELF loader-cache candidates, sanitized Python metadata and Kerf source
pin status. It exports no full configuration, environment, primary state or raw
command stderr. Authenticated Python direct-reference URLs are reduced to
requirement names and counts. Unknown license metadata remains review-required.

Use the separate aggregate exporter for retained proof:

```bash
sudo python3 -I -S scripts/summarize-runtime-dependencies.py \
  /var/lib/mkruntime/dependency-reviews/inventory.json
```

Its fixed schema includes counts, booleans, bounded gap categories and the
private report digest. It excludes software names, versions, internal artifact
paths and license text. An inventory with gaps still reports
`INVENTORY_COLLECTED_REVIEW_REQUIRED`; zero hash mismatches do not resolve its
other findings. Retain exact collector/inspector bytes and terminal collection
receipts so the report digest is tied to an identified run.

Review machine-readable copyright labels, missing project licenses, vendored
versions omitted from build information, source/package provenance and actual
advisory reachability separately. Loader-cache candidates do not establish
runtime dlopen/import policy or custom kernel/host-child ABI correctness. Feed
these inventory/provenance inputs into G9's release automation; this does not
close G9 or all G7 dependency acceptance.

The [qualified imported-code correction and private-inventory proof](../../evidence/runtime-20261012/g7-kerf-import-code-trust/README.md)
retains the actual group-write control, permission-only correction, real
ctr/Docker lifecycle markers, failed warm-pool assertion and successful idle
restart/resource return. It preserves remaining review gaps and full-report
privacy rather than treating inventory completion as security acceptance.
