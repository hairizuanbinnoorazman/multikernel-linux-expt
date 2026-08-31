# Configuration contract (v1)

Host configuration is root-owned JSON, mode 0640 or stricter. Environment
variables may select a test config path but may not override security policy.

Required host fields are state directory, Unix socket, Kerf executable,
Multikernel sysfs root, approved kernel-manifest directory, minimum primary
CPU count and memory, forbidden APIC IDs (including 0), backend timeout, and
maximum protocol frame size. Defaults are `/var/lib/mkruntime`,
`/run/mkruntimed.sock`, `/sys/fs/multikernel`, four primary CPUs, 8 GiB primary
memory, 30 seconds, and 1 MiB.

Per-sandbox input contains only ID, requested CPU APIC IDs, memory bytes,
approved kernel-manifest name, OCI bundle path, child CID, agent endpoint, and optional
labels. CPU IDs are unique non-negative integers and may not intersect the
forbidden set. Memory is an integer byte count. Labels have bounded key/value
sizes and never affect filesystem paths.

Configuration is parsed strictly. Unknown fields, duplicate JSON keys,
relative artifact paths, numeric overflow, world-writable configuration, and
unsafe state/socket parents are rejected before mutation.
