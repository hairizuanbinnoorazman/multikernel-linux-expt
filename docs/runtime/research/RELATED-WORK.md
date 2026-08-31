# Related-work audit

Checked on 2026-08-30 using GitHub repository and code search.

## Exact implementation search

No public implementation was found for any of the following:

- a Multikernel Linux containerd Runtime v2 shim;
- a Kerf-backed OCI or CRI runtime;
- a Multikernel Kubernetes `RuntimeClass` implementation; or
- a Kata Containers Multikernel backend.

## Closest projects

- [multikernel/kerf](https://github.com/multikernel/kerf) implements resource
  pools and child-kernel lifecycle. It is the selected backend.
- [multikernel/kmorph](https://github.com/multikernel/kmorph) describes runtime
  management and lists a Kubernetes operator as future work, but the repository
  contained only a README, license, and image at the audit date.
- [multikernel/multikernel.io](https://github.com/multikernel/multikernel.io)
  advertises Docker-image booting and Kubernetes integration. No corresponding
  public shim, operator, or OCI runtime was found in the organization.
- [AkihiroSuda/multikernel-test](https://github.com/AkihiroSuda/multikernel-test)
  runs Multikernel Linux under Lima in GitHub Actions; it is test automation,
  not a container runtime.
- [Kata Containers](https://github.com/kata-containers/kata-containers) provides
  the closest container-facing architecture, but its supported VMMs use KVM.
- [Firecracker](https://github.com/firecracker-microvm/firecracker) is a useful
  reference for lifecycle discipline, process isolation, metrics, and negative
  testing, but its execution model is KVM-specific.

Repeat this audit before publishing a major design or claiming novelty.
