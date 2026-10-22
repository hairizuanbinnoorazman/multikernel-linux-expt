# Guest service process scope

This Linux package supplies the cgroup ownership primitive for a supervised
Docker engine inside a child kernel. It is used by the explicit `mk-agent --docker-guest` supervisor path;
protected production boot preparation is implemented. The
[direct-child build proof](../../../evidence/runtime-20261022/g8-production-docker-build/README.md)
qualifies the supervised profile with mediated storage; complete host Docker
lifecycle, recovery and adversarial gates remain separate.

`New` exclusively creates a protected cgroup-v2 leaf under a protected real
parent. Memory/pids controllers must already be enabled by trusted boot setup.
It applies finite memory and PID limits, disables accounted swap, enables group
OOM handling, and requires `cgroup.kill` before admitting any process. It never
adopts an existing group. A zero-value scope has no authority.

`Start` uses `clone3(CLONE_INTO_CGROUP)` through Go's `UseCgroupFD`; the process
belongs to the group before executing child code. Admission holds a lock through
clone, so shutdown cannot close/reuse the descriptor during admission. Credentials
and other caller-selected process attributes are preserved. The integrating
supervisor must supply its fixed command, trusted environment and credentials.

`Stop` blocks further admission, kills all descendants including independent
sessions, confirms `populated 0`, checks directory identity and removes only its
own leaf. Waiting has a 30-second ceiling. A canceled/failed/timeout release is
not success and retains ownership for a bounded retry. Completed release is
idempotent. It never walks or repairs a foreign cgroup tree.

Unprivileged tests cover policy/event validation and zero-value authority.
Opt-in installed-kernel tests use `MK_TEST_GUEST_SCOPE_PARENT` and create only
fresh PID-specific groups. They check session escape containment, canceled-stop
behavior, idempotence, outside-process survival and actual cgroup OOM enforcement.
A passing primary-kernel test is not a child Docker workload qualification.

The [Docker guest contract](../../../docs/runtime/contracts/docker-guest-profile.md)
and [deployment constraint](../../../docs/runtime/architecture.md#deployment-and-containment-constraint)
still apply. Guest process resource controls do not establish containment after
arbitrary child-kernel compromise; nested virtualization is unavailable in the
target cloud instances.

`ContainsLivePeer` checks a socket-provided live peer pidfd and direct cgroup
membership with a bounded population read. The supervisor obtains both peer
credentials and pidfd from the same Unix socket; numeric PIDs alone are not
authority. No nested cgroup delegation is permitted.
