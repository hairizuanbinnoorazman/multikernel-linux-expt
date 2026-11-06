# Health diagnostic bundle (explicit version 2)

```sh
sudo install -d -m 0700 /var/lib/mkruntime-diagnostics
sudo mkruntimed --config=/etc/mkruntime/config.json \
  --network-socket=/run/mknetd.sock \
  --diagnostic-bundle=/var/lib/mkruntime-diagnostics/incident-health-001.json \
  --diagnostic-format=2 \
  --diagnostic-max-files=32 --diagnostic-max-bytes=67108864
```

Version 2 has scope `operational-health-and-component-observation-snapshot`.
Its snapshot contains an independent compiled-component report and the complete
[version-8 health observation](operational-health.md). Component labels are
sampled first so resource/API failures do not discard those diagnostics. Both
reports share the original configured caller budget; they are separate sampled
observations, not an atomic cross-daemon view.

The receipt also has version 2 and this scope. Its aggregate observation status
includes both sections: degraded takes precedence over inconclusive, then
consistent. Failed journals or degraded recorded network CHECKs cannot be hidden
by consistent ownership or build labels. Pending work and unavailable observations
remain inconclusive; cancellation does not become an empty healthy inventory.
Exit 0 requires publication and consistent aggregate observations; saved troubled
observations exit 1. Receipt bytes/SHA-256 describe the exact private artifact.

The typed inspector callback is trusted internal code using the validated,
redacting doctor APIs. Capture checks report discriminators, supported statuses,
complete-vs-unavailable payload shape, health severity coherence and false
attestation/admission flags; it does not authenticate arbitrary caller-supplied
JSON or reimplement every producer correlation. A receipt proves publication,
not the truth of a compromised producer, readiness or containment.

Version 1 stays the default and retains its existing snapshot/receipt contracts.
An explicitly supplied diagnostic format requires a bundle destination; only 1
and 2 are accepted. Invalid arguments exit 2 before configuration load. Doctor
format selection is separate and cannot select a diagnostic bundle format.

Both versions use the same [protected publisher and optional directory budget](diagnostic-bundle.md):
existing caller-owned mode-0700 parent, no symlink traversal, new canonical JSON
name, mode-0600 exclusive publication, exact receipt hash, 8 MiB encoded cap,
file/directory synchronization and nonblocking directory-inode locking. Mixed
versions count toward the same opt-in file/logical-byte limits. Existing evidence
is never overwritten, rotated or deleted. Limits must be supplied on each bounded
capture; they are not a persisted/node-wide quota or space reservation.

Unsupported/canceled/degraded observations can still be saved after the inspection
deadline. Local filesystem synchronization retains its separate latency contract.
No raw logs, guest output, credentials, environment, configuration/state files or
kernel console are collected. Identifiers and opaque fingerprints remain protected
operator material, not an automatically sanitized public export.

Service readiness, current journal writability/bytes, executable/kernel provenance,
end-to-end network/Docker health, recovery and containment remain unobserved as
defined by the producer. Full G7/G8 remain open. Direct child kernels need no
nested virtualization and share host trust after compromise.
