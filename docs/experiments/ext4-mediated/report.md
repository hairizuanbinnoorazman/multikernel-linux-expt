# Primary-mediated ext4 child-root implementation

Execution date: 2026-08-30

## Result

Alternative approach 2 in the
[direct ext4 plan](../ext4-direct/plan.md) passed its
decisive functional test on a new GCE VM. The primary retained the shared GCE
virtio-SCSI controller and mounted one 20 GiB Persistent Disk as outer ext4.
Two fixed-size image files on that filesystem were exported through separate
primary processes, Multikernel AF_VSOCK endpoints, and child-local NBD
adapters. Each child mounted its own image as read-write ext4 `/`.

The simultaneous result was:

```text
mklinux-mediated-20260830 (n2-standard-16)
├── primary 7.0.0-mk2-gce-lab
│   └── /dev/sdb ext4 UUID 507c0523-… at /srv/multikernel-storage
│       ├── child-a/root.ext4 -> server port 4061
│       └── child-b/root.ext4 -> server port 4062
├── child A 7.0.0-mk2-gce-lab
│   └── /dev/nbd0 ext4 UUID 352be7e6-… as /
└── child B 7.0.0-mk2-gce-lab-alt
    └── /dev/nbd0 ext4 UUID 0fc9d98e-… as /
```

Kerf captured both children `active` at the same time. Their device trees had
only CPU and memory resources: no PCI, SCSI, NVMe, NIC, or physical storage
device was assigned. Child A's durable counter advanced from 1 through 7
across repeated child recreation, a GCE reset, and a full GCE stop/start.
Child B advanced from 1 to 2 across the repeated dual run.

## Cloud and artifact ledger

| Item | Value |
| --- | --- |
| Project / zone | `new-demo-project-462517` / `asia-southeast1-b` |
| VM | `mklinux-mediated-20260830`, `n2-standard-16` |
| Boot source | `mklinux-lab-pre-daxfs-20260828-2030` snapshot |
| Boot disk | `mklinux-mediated-boot-20260830`, 100 GiB `pd-balanced`, auto-delete enabled |
| Storage disk | `mk-mediated-storage-20260830`, 20 GiB `pd-balanced`, auto-delete disabled |
| Outer ext4 | label `mk-mediated-host`, UUID `507c0523-8e58-4ae3-9524-3b7513aad344` |
| Child A ext4 | label `mk-child-a-root`, UUID `352be7e6-ef7c-44d2-9f42-fe84303cddf4` |
| Child B ext4 | label `mk-child-b-root`, UUID `0fc9d98e-3902-44a1-b4be-7a648fd28e46` |
| Image size/allocation | 4,294,967,296 logical bytes and at least 4,294,971,392 allocated bytes each |
| Primary kernel | pinned `3bdd35b6…`, `7.0.0-mk2-gce-lab` |
| Alternate kernel | pinned `3bdd35b6…`, `7.0.0-mk2-gce-lab-alt`, SHA-256 `ebddd00e…69cf2` |
| Final NBD helper | SHA-256 `b7b5e861…d05e9` in the decisive dual run |
| Primary transport module | SHA-256 `f7bcaf7f…b23495f` |
| Alternate transport module | SHA-256 `2c746e01…c12fae` |

At evidence capture, the VM was stopped (`TERMINATED`) and both disks remained
attached. The VM was subsequently deleted: its 100 GiB auto-delete boot disk
was removed, while the detached 20 GiB storage disk and image files were
deliberately retained and remain billable.

## Transport and block design

The pinned tree contains `CONFIG_MULTIKERNEL_VSOCKETS`, but it was disabled in
the saved config and did not compile when first enabled. The callback type of
`stream_allow` was stale relative to the same pinned kernel. The checked patch
[`linux-v7.0-mk2-vsock-build-safety.patch`](../../../patches/linux-v7.0-mk2-vsock-build-safety.patch)
fixes that build break and rejects inconsistent receive lengths before an skb
is queued.

The bounded transport proof echoed deterministic payloads of 1, 63, 64,
4,095, 4,096, 4,097, 16,384, 32,768, 65,536, and 1,048,576 bytes. It passed in
both directions without a device assignment.

Linux NBD then rejected a direct AF_VSOCK file descriptor with
`Unsupported socket: should be TCP or UNIX`. The static `mkvsock-nbd` helper
therefore creates an isolated child-loopback TCP socket for the NBD ioctl and
forwards only that stream to the authenticated AF_VSOCK connection. It does
not use or assign a NIC. The server:

- obtains a nonblocking exclusive `flock` on one named image;
- authenticates protocol version, image ID, generation, and exact size before
  handing any block traffic to NBD;
- bounds offsets and request sizes;
- implements read, write, flush, and FUA, and returns `EOPNOTSUPP` for other
  commands;
- uses positional I/O, propagates errors, never replays a partial write, and
  calls `fdatasync` before closing; and
- exposes one listener, server, port, identity, and generation per child.

The bootstrap loads only the patched transport and stock NBD modules, enables
only loopback, connects to the expected export, verifies the ext4 UUID by
reading the superblock, mounts `/dev/nbd0`, validates the child marker, and
uses `switch_root`.

## Executed gates

| Gate | Result |
| --- | --- |
| Fresh restored VM and host health | Pass |
| Blank disk identity and guarded outer format | Pass; rerun refused |
| Outer ext4 mount and offline check | Pass |
| Multikernel VSOCK build | Initial fail; passed with recorded patch |
| Transport boundary/integrity proof | Pass through 1 MiB |
| Fully allocated inner images | Initial allocation audit caught discard-created holes; fixed by post-format reallocation and assertion |
| Direct AF_VSOCK to NBD | Expected fail; kernel accepts only TCP/Unix sockets |
| Loopback TCP-to-VSOCK adapter | Pass |
| One child ext4 root | Pass, repeated three times before dual test |
| Two distinct concurrent kernels/roots | Pass twice; first teardown exposed and then fixed partial-write handling |
| Cross-export isolation | Pass by distinct endpoint identity, UUID, marker, and absence of the peer UUID in each console |
| Primary reset persistence | Pass after GCE reset; counter advanced to 6 |
| GCE stop/start persistence | Pass; counter advanced to 7 |
| Final resource return | Pass; no instances/pool, CPUs `0-15`, guest agent active |
| Final inner/outer `e2fsck -fn` | Pass with return code 0 for both inner filesystems and the outer filesystem |

## Important failures and learnings

1. Source presence did not imply buildability. The pinned VSOCK transport had
   an in-tree API mismatch and requires the recorded patch.
2. Static musl did not provide `linux/vm_sockets.h`; the helper embeds the
   stable small sockaddr/socket-option ABI it needs.
3. A server must not set an accept timeout before the comparatively slow pool,
   image-load, and child-start sequence. The first dual run timed both listeners
   out before either child connected.
4. NBD deliberately restricts its socket family. The child loopback adapter is
   required; there is no direct AF_VSOCK NBD attachment at this revision.
5. This BusyBox build has no `blkid` applet. The initial UUID check failed
   safely; the final bootstrap uses a read-only ext4 superblock UUID reader.
6. `mkfs.ext4` discard through a loop device punched holes in the initially
   preallocated outer file. Re-running `fallocate` after formatting restores
   full allocation without overwriting initialized extents.
7. A forced child stop does not promptly signal VSOCK connection closure to
   the primary. The server uses a bounded accepted-socket timeout and final
   sync. One dual teardown stopped B during a 2 MiB write payload; the server
   recorded and discarded the incomplete request, never replayed it, synced
   earlier completed writes, and both filesystems passed offline checks.
8. `systemctl reboot` was accepted and logged by the custom primary but did
   not restart it. A GCE reset changed the boot ID and completed the reboot
   persistence test. A later GCE stop/start also passed.
9. Forced child teardown leaves the ext4 journal marked for recovery even
   after a successful NBD flush. `e2fsck -fn` reported skipped journal replay
   but no structural errors and returned 0. A clean child shutdown protocol is
   still required before treating this as production-ready.
10. The upstream transport still advertises space unconditionally and lacks
    production-grade credit/backpressure, authentication, and connection-state
    semantics. The application protocol bounds requests, but this experiment
    is a functional proof, not a production storage service.

## Acceptance boundary and remaining work

The core Approach 2 objective passed: two different child kernels ran
concurrently with isolated, persistent, child-mounted ext4 roots while the
primary retained every GCE controller. The following plan items remain open:

- clean child-driven read-only remount, NBD disconnect, and shutdown;
- malformed transport-packet injection below the AF_VSOCK API;
- sustained saturation/backpressure and performance testing;
- outer high-water/ENOSPC refusal under live I/O;
- server failure during read, write, and flush on disposable image copies;
- wrong/stale generation and duplicate-lock live negative tests;
- damaged-image isolation and offline snapshot/clone recovery; and
- a reviewed service manager and production authentication/authorization
  design.

Raw evidence is indexed in
[`evidence/ext4-mediated-20260830/README.md`](../../../evidence/ext4-mediated-20260830/README.md).
