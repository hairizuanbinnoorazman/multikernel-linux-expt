# G4-G6 guest I/O live evidence

Disposable GCE instance `mklinux-g4-g6-io-20260901` ran in
`asia-southeast1-b` on `n2-standard-16`, with a 100 GB auto-delete boot disk
restored from `mklinux-lab-pre-daxfs-20260828-2030`. The read-only host report
qualified `7.0.0-mk2-gce-lab` before the runtime was exercised.

The final shared matrix passed through both `ctr` and Docker, including the new
guest-I/O rows:

- foreground guest stdin and ordered `CloseIO` EOF delivery;
- detach followed by live stdin/output reattachment;
- real PTYs for terminal processes; and
- propagation of an initial 91-column by 37-row size, asserted inside each
  guest as `37 91`.

The harness set the host PTY size before starting each task and did not change
it after the guest process was confirmed running. This run therefore does not
prove a deliberate post-start live resize.

The first live stdin probe exposed a `CloseIO` ordering race, and the first
strict terminal probe exposed missing guest `devpts` setup. The retained final
transcript is from the corrected build and ends with
`G4_G6_CTR_DOCKER_FEATURE_MATRIX_PASS` and command exit status 0.

Evidence:

- [`g4-g6-io-final.log`](g4-g6-io-final.log): final marker-oriented matrix; it
  does not retain the asserted stdin/attach strings or `37 91` values.
- [`g4-g6-io-environment.log`](g4-g6-io-environment.log): versions, revisions,
  component hashes, image digest, active services, and empty final inventories.
- [`g4-g6-io-host-report.json`](g4-g6-io-host-report.json): qualified host
  report.
- [`cloud-cleanup.txt`](cloud-cleanup.txt): deletion and absence verification
  for the disposable VM and auto-delete boot disk.
- [`manifest.json`](manifest.json): assertions mapped to the retained files.

The manifest is a non-conforming historical index: component versions are
missing, the redacted boot ID is invalid under the current schema, the dirty
source diff is absent, and `gate: G6` plus `result: pass` is scoped too broadly
for this shared feature run.

Pause/resume, `Stats`, `Update`, `Checkpoint`, faithful guest PIDs, CNI, the
full G4 storage matrix, and shim-crash task reconnection remain outside this
pass.
