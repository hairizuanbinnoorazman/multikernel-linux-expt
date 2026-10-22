# Primary network broker privilege contract

The managed mknetd service runs as root with NoNewPrivileges=yes and the
following capability bound. Its helpers inherit the same bound. Ambient
capabilities are explicitly empty. Root-owned socket/state files are accessed
as their owner; filesystem override, ownership-changing, identity-changing,
module/reboot/raw-device-I/O and other unrelated capabilities are excluded.

| Capability | Current operation requiring or preserving it |
| --- | --- |
| CAP_NET_ADMIN | Create/move veth devices, configure addresses/routes/TUN interfaces and IPv4/IPv6 firewall policy; attach the endpoint TUN |
| CAP_SYS_ADMIN | Create/delete named network namespaces, namespace bind mounts and enter/restore network namespaces with setns/nsenter |
| CAP_SYS_PTRACE | Open proc-based CNI namespace paths belonging to other privileged root processes; required by the proc/ptrace access check |
| CAP_NET_RAW | Preserve the configured iptables interface's legacy raw-socket backend compatibility; not proven necessary for the tested nf_tables backend |

The expected effective, permitted and bounding mask on Linux is 0x283000.
Inheritable/ambient sets remain empty. A disposable-host read-only control
shows that 0x203000 without ptrace cannot open the existing root containerd
namespace (EACCES), while 0x283000 can. This proves the namespace-file access
requirement, not all ptrace uses or a universal minimum for every tool version.

The [Linux namespace API](https://www.man7.org/linux/man-pages/man2/setns.2.html)
requires namespace administration authority. Linux documents
[capability operations](https://www.man7.org/linux/man-pages/man7/capabilities.7.html);
[systemd's capability contract](https://github.com/systemd/systemd/blob/main/man/systemd.exec.xml)
removes unlisted capabilities and applies the bound to service execution.

SYS_ADMIN and SYS_PTRACE retain broad primary-host authority. This is a
capability reduction, not containment of a compromised broker. The root
filesystem owner can also modify root-owned writable files. Further reduction
requires splitting namespace/process-handle operations or changing the CNI
handoff, then qualifying that design. The main runtime daemon remains a separate
privileged component. Whole G7 SEC-10 and the G8 privilege gate remain open.

The [Docker service-policy qualification](../../../evidence/runtime-20261022/g8-host-docker-service-policy/README.md)
now runs private candidate runtime/network processes with the declared capability,
NNP and ambient settings read from these committed service units. Actual process
masks and executable/unit hashes match before host Docker builds and after the
runtime restart; workload continuity and teardown pass. This proves compatibility
of those declared bounds, not capability minimality or every systemd restriction.
The deployment uses direct children without nested virtualization; arbitrary
child-kernel compromise shares physical host trust.
