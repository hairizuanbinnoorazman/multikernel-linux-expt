# G4-G6 robustness follow-up

## Result

The 2026-09-01 follow-up on disposable GCE instance
`mklinux-gates-20260901` remains **provisional**, but closes several concrete
fresh-host and recovery gaps. The host used the qualified
`7.0.0-mk2-gce-lab` kernel, an `n2-standard-16` machine, and an auto-delete
100 GB boot disk restored from `mklinux-lab-pre-daxfs-20260828-2030`.

Clean isolated reruns proved:

- containerd restart reconnected to a running shim and retained the same child
  boot ID;
- `mkruntimed` restart retained the same running child and boot ID;
- an injected initramfs-builder ENOSPC error failed creation before allocating
  a child or network resource;
- forced shim death safely reclaimed the Kerf child, memory pool, TUN device,
  NAT rule, and forwarding rules after network recovery identity was persisted.

## Fresh-host fixes

The snapshot deliberately lacked runtime packages and image caches. Recreating
the service exposed bootstrap assumptions that the earlier warm-host MVP did
not catch:

- `make sync` omitted `guest/mk-agent-init`;
- `/sys/fs/multikernel` was not mounted by a persistent unit;
- Docker needed the Runtime v2 `runtimeType` registration rather than a
  runc-compatible `path` registration;
- the baseline proof assumed BusyBox was already present in both image stores.

The sync list, systemd mount dependency, Docker example configuration, and
explicit image pulls now cover these conditions.

## Remaining formal failures

- Terminal task creation is rejected before child allocation. Consequently
  terminal resize cannot yet be exercised and remains unsupported.
- Forced shim death now has bounded, leak-free safe reclaim, but the running
  task is not reconstructed or reconnected. Safe reclaim is not equivalent to
  the G6 reconnect requirement.
- No CNI configuration or Multikernel CNI binary was installed; normal CNI
  `ADD`, `CHECK`, and `DEL` remain unimplemented.
- The injected ENOSPC rollback covers only one storage failure point. Metadata,
  malformed-image variants, inode/block exhaustion, high-water refusal,
  persistence, corruption, and recovery still require the full G4 matrix.

The final inventory contained no Multikernel instances, containerd tasks or
containers, Docker containers, `mkn*` links, Multikernel NAT/forwarding rules,
or temporary systemd environment overrides.
