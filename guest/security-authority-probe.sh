#!/bin/busybox sh
# Diagnostic workload: read inventories and touch only explicit synthetic canaries.
# Never print kernel/process command lines, environments, or credential values.
set -eu
bb=/bin/busybox
echo SECURITY_AUTHORITY_BEGIN
$bb id
$bb uname -r
for field in Uid Gid Groups CapInh CapPrm CapEff CapBnd CapAmb NoNewPrivs Seccomp; do
    $bb grep "^$field:" /proc/self/status || true
done
if $bb grep -q 'mk.token=' /proc/cmdline 2>/dev/null; then
    echo KERNEL_COMMAND_LINE_TOKEN_VISIBLE=yes
else
    echo KERNEL_COMMAND_LINE_TOKEN_VISIBLE=no
fi
agent_argument_visible=no
for path in /proc/[0-9]*/cmdline; do
    if $bb grep -q -e '--token-hex=' "$path" 2>/dev/null; then
        agent_argument_visible=yes
    fi
done
echo AGENT_ARGUMENT_TOKEN_VISIBLE=$agent_argument_visible
for path in /dev/mem /dev/kmem /dev/port /dev/nbd0 /dev/kmsg /dev/multikernel \
    /proc/modules /proc/sys/kernel/modules_disabled /proc/sys/kernel/yama/ptrace_scope \
    /run/multikernel-agent/control.sock /proc/1/root/run/multikernel-agent/control.sock; do
    exists=no; readable=no; writable=no
    [ ! -e "$path" ] || exists=yes
    [ ! -r "$path" ] || readable=yes
    [ ! -w "$path" ] || writable=yes
    echo "PATH_AUTHORITY path=$path exists=$exists readable=$readable writable=$writable"
done
echo MOUNTS_BEGIN
$bb cat /proc/self/mountinfo
echo MOUNTS_END
echo DESCRIPTORS_BEGIN
for fd in /proc/$$/fd/*; do
    echo "FD ${fd##*/} $($bb readlink "$fd" || true)"
done
echo DESCRIPTORS_END
# The marker lives in the child bootstrap, outside the OCI chroot. It is not
# a primary-host file and is never a real secret. Limit access to this exact path.
canary=/proc/1/root/security-canary
if [ "$($bb cat "$canary" 2>/dev/null || true)" = MK_SYNTHETIC_BOOTSTRAP_CANARY ]; then
    echo CHILD_BOOTSTRAP_CANARY_READ=yes
else
    echo CHILD_BOOTSTRAP_CANARY_READ=no
fi
if (printf '%s\n' MK_SYNTHETIC_WORKLOAD_WRITE > "$canary") 2>/dev/null; then
    echo CHILD_BOOTSTRAP_CANARY_WRITE=yes
else
    echo CHILD_BOOTSTRAP_CANARY_WRITE=no
fi
echo SECURITY_AUTHORITY_END
