# Guest native transport boundary

The trusted agent, native relay and NBD client keep their transport authority.
Workload executors install this fixed seccomp filter before user code. Docker's
owned bootstrap installs it before its approved script, mapping helpers and
daemon; it uses bootstrap CAP_SYS_ADMIN without setting NNP prematurely.
Descendants retain the policy across exec and user namespace changes.

The audited amd64 policy denies AF_VSOCK socket/socketpair creation and all
io_uring entry points, which could otherwise create sockets outside the socket
syscall check. Foreign and x32 ABIs terminate the process. Other native syscalls,
including Unix and Internet sockets, retain their ordinary kernel permissions.
This does not provide a general workload syscall allowlist or network policy.

Installation requires successful seccomp TSYNC across all threads. Unsupported
architectures, unavailable privilege/NNP or synchronization failure refuse the
launch. Trusted control processes are not filtered by this package. The locked executor also closes inherited descriptors with close_range(UNSHARE)
immediately before exec. Workloads retain only stdio; the bootstrap additionally
retains its verified regular script at fd3. Descriptor-table unsharing prevents
other runtime threads from repopulating the exec table.

Tests execute real kernel socket/io_uring checks in separate processes, verify
exec inheritance and x32 refusal, and preserve ordinary Unix/Internet sockets.
Full child Docker startup/build and retained-descriptor boundaries still require
qualification. Direct-child kernels use no nested virtualization; arbitrary
child-kernel compromise remains outside this process boundary.
