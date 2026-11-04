"""Exact same-boot module taint transitions for disposable cold trials."""

MODULE_ORIGIN_BITS = (1 << 12) | (1 << 13)  # externally built / unsigned module


def expected_module_taint(before, after):
    """Return the exact post-load taint; reject cleared or unexpected new bits.

    Existing taints remain observable. Only the O/E origin bits may be added by
    loading the pinned trial module. This does not authorize any other warning,
    oops or integrity taint, or qualify the original module after a cold reboot.
    Callers must independently bind the boot and module identities.
    """
    if any(type(value) is not int or not 0 <= value < 1 << 64 for value in (before, after)):
        raise ValueError('invalid module taint observation')
    if before & ~after:
        raise ValueError('module taint bits cleared within the observed boot')
    if (after & ~before) & ~MODULE_ORIGIN_BITS:
        raise ValueError('unexpected taint added during module trial')
    return after
