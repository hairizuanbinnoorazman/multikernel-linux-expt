# Shim service entry errors

The binary wraps its inspection/service initializer before passing it to the
pinned containerd shim runner. The runner prints returned initialization and
manager errors directly to stderr, outside the logrus boundary. Service-origin
failures at this entry point now contain only one of three fixed descriptions:
`shim initialization failed`, `shim manager startup failed`, or
`shim manager cleanup failed`. No private cause, unwrap chain, partial address
or failed cleanup response crosses this boundary. Causes are never formatted.

The wrapper delegates once with the original context/options. Successful manager
addresses, cleanup protobufs and all promoted Task methods retain their existing
contracts. The reusable Task facade and internal service callers still receive
their previous manager errors. The runner returns through its ordinary shutdown
and publisher defers before its existing terminal exit; the wrapper does not
call `os.Exit` or replace stderr/FIFO descriptors.

This is a terminal service-entry boundary, not a new RPC status translation.
The selected failures still exit 1. The runner's earlier flag, signal/publisher
setup, logger, socket and other errors remain outside this scope. Its direct
stderr formatter also retains its existing blocking behavior: full-stderr
budget safety is not qualified here. Successful Task payloads, child containment
and complete G7/G8 remain separate. Direct children use no nested virtualization
and share primary trust after child-kernel compromise.
