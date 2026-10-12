#!/usr/bin/env python3
"""Reproduce the pinned-source identity and anchors for SEC-01/SEC-11.

This verifies reviewed bytes and emits source excerpts. It does not infer the
absence of a hardware boundary merely from an unsuccessful keyword search.
Fetch files from the recorded immutable raw URLs into SOURCE before running.
"""
import argparse
import hashlib
import json
from pathlib import Path

REVISION = "3bdd35b64413da0b4e089ce931bfc2e8b031cbf7"
FILES = {
    "arch/x86/multikernel/direct_boot.S": ("7b0b5ccda22591986ea73baf8cb2166f4a1513663cca13b39e5f924a8472e89e", ["SYM_CODE_START_NOALIGN(multikernel_relocate_kernel)", "pushq\t$__KERNEL_CS", "jmp\t*%r14"]),
    "arch/x86/multikernel/e820.c": ("ca374ca55792a8e97500144f8bd5eee82c22fb3af9fb2005aadb0ce3be6203e2", ["list_for_each_entry(region", "E820_TYPE_RAM"]),
    "arch/x86/multikernel/spawn.c": ("6a7f5412afa43f36fb645f3c457e6135b7cbd741623707ec5b8880b8cc48c883", ["pgt = mk_build_identity_pgtable(instance)", "mk_ident_map_range(pgt", "machine_ops.power_off = mk_machine_halt"]),
    "kernel/multikernel/mem.c": ("07c0436128eb7a3d91437589b682d3dd4d65e51e75f727edf2d5d8030b8f68cb", ["phys_addr = multikernel_alloc(size, node)"]),
    "kernel/multikernel/hotplug.c": ("de0633bb71bb318aab8278db4ac9cf4418cd7f8bb494f0537b75b8f04784c7eb", ["ret = mk_do_cpu_remove(cpu_id)", "MK_RES_ACK"]),
    "kernel/multikernel/ipi.c": ("4154dc981fb3c4cc57c1c383e73fdd2a2d0231089f5df5bda23fb1add02d0fe1", ["return i & (MK_IPI_RING_SIZE - 1)", "memcpy"]),
    "kernel/multikernel/dma_heap.c": ("5b87403b1672efcaa8b21b3ed287b1ef174b26de6a472e77a0763d9f90aadfcb", ["return remap_pfn_range", "dma_map_sgtable"]),
    "kernel/multikernel/core.c": ("74ba0a63d8f761683a74a81503d9ea27c3871fa7d0e4268a95b26b0c7e1d76ba", ["mk_halt_to_pool"]),
    "net/vmw_vsock/mk_transport.c": ("37a9e02cff4e08688a3a7be33fefbc2aa7b6efc86858fc27768e5282c3240c59", ["skb_put_data"]),
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path)
    args = parser.parse_args()
    files = []
    for name, (expected, anchors) in FILES.items():
        data = (args.source / name).read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest != expected:
            raise SystemExit(f"reviewed source hash mismatch: {name}")
        lines = data.decode().splitlines()
        excerpts = []
        for anchor in anchors:
            matches = [i for i, line in enumerate(lines) if anchor in line]
            if not matches:
                raise SystemExit(f"review anchor missing: {name}: {anchor}")
            index = matches[0]
            first, last = max(0, index - 3), min(len(lines), index + 6)
            excerpts.append({"anchor": anchor, "first_line": first + 1,
                             "lines": lines[first:last]})
        files.append({"path": name, "sha256": digest, "size_bytes": len(data),
                      "url": f"https://raw.githubusercontent.com/multikernel/linux/{REVISION}/{name}",
                      "excerpts": excerpts})
    print(json.dumps({"revision": REVISION, "reviewed_file_count": len(files),
                      "hashes_verified": True, "files": files,
                      "scope": "Identity and source anchors; architectural conclusions require source review."}, indent=2))


if __name__ == "__main__":
    main()
