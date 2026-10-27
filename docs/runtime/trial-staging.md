# Prepare a disposable Docker trial packet

Use the committed [packet builder](../../scripts/build-runtime-trial-stage.py)
from a trusted checkout. It prepares artifacts only. Boot/kernel/image approval,
idle-primary checks, protected systemd scope, helper selection, storage admission
and measured recovery remain requirements of the cold-trial driver.

Prerequisites are Git, Python, the Go toolchain selected by `runtime/go.mod`,
its dependencies, and the pinned containerd/helper artifacts. Containerd must
match `dbc252c7f4e3534dcbe674552b87b20194967e7b6069e66de61632dcdd3acc83`.
The helper must match the selected variant in the committed
[trial helper pins](../../scripts/runtime_trial_helper.py). The qualified helper
remains the default; the server-queue candidate requires explicit selection.

For the candidate, supply the materialized static artifact and a new output path:

```bash
python3 scripts/build-runtime-trial-stage.py \
  --output /tmp/mk-trial-stage \
  --containerd /path/to/qualified/containerd \
  --helper /path/to/pinned/mkvsock-nbd \
  --helper-variant server-queue-candidate
```

The default revision is HEAD; `--revision` selects another commit. Runtime code
and helper pins come from that commit's source archive. Dirty working-tree files
are excluded. Builds run inside the extracted snapshot with CGO disabled,
readonly modules, trimpath, GOWORK disabled and ambient GOFLAGS cleared. The
selected commit, source/archive/binary hashes and actual Go build information
are recorded in `build.json`.

The output contains `source.tar`, its private extracted tree, five binaries,
`build.json` and `stage.tar.gz`. The stage root, source root and bin directory
are mode 0700; binaries are 0500 and metadata/packets are 0600. Packet entries
have normalized root ownership. This prevents the prior 0775-bin staging error
under umask 0002. Git source archives explicitly use tar.umask=0022 so their
nested directories/files also retain protected modes after extraction. Existing output generations, unsafe parents, source symlinks,
and mismatched inputs refuse. Failed generations remain private and lack a
completed packet; choose a new output path after inspecting the failure.

Transfer `stage.tar.gz` to the GCE lab with gcloud and verify its digest against
the builder's JSON output before privileged extraction. Use a new root-owned
`/opt/mkruntime/g8-host-<short-commit>` directory at mode 0700. Extract the packet,
then create its `source` directory at mode 0700 and extract `source.tar` there.
Verify source identity, all binary hashes, root ownership and protected modes.
Extraction must preserve the pinned archive's exact modes. Python tarfile's
`data` filter changes directory/file modes, including private bin directories
and read-only executable artifacts; it is insufficient for this packet contract.
For a hash-verified trusted packet, use an extractor that retains permissions
and ownership, then verify every member before launch. Never infer safe modes
from successful extraction alone.
The cold trial's revision must match the stage; candidate trials explicitly set
`MK_MODULE_TRIAL_HELPER_VARIANT=server-queue-candidate`. The driver forwards the
same variant to its private Docker harness. No helper is selected by guessing its
hash or by changing production defaults.

[Real packet proof](../../evidence/runtime-20261027/g8-trial-stage-builder/README.md)
covers committed-source builds despite dirty/ambient inputs, refusal boundaries,
packet extraction on GCE, protected outer modes, binary/source hashes and unchanged
watched primary state. It does not qualify a child Docker run. The [approved data-disk expansion](../../evidence/runtime-20261027/g8-data-disk-expansion/README.md)
now provides 50 GB with all retained files preserved. Fresh builder admission
still applies; failed images and the 2 GiB image/1 GiB reserve remain intact.
The later [nested-source mode fix](../../evidence/runtime-20261027/g8-source-archive-modes/README.md)
qualifies every source member's protected mode. The fresh db62c25 stage passes
[actual child Docker pressure/build/normal cleanup](../../evidence/runtime-20261027/g8-live-static-candidate/README.md)
and [separate cold restoration](../../evidence/runtime-20261027/g8-live-static-restoration/README.md).
Installed default helper pins remain unchanged; full G7/G8 remains open.

Direct child kernels use no nested virtualization. The shared host trust boundary
after child compromise and the remaining full G7/G8 requirements still apply.
