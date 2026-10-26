# Runtime design documentation

- [`status.md`](status.md): proved results, current client feature matrix, and
  safety boundary.
- [`architecture.md`](architecture.md): target components, lifecycle, and trust
  boundary.
- [`decisions/`](decisions/): architecture decision records.
- [`research/`](research/): related-work audits.
- [`research/stride-threat-model.md`](research/stride-threat-model.md): STRIDE
  threats, workload-escape hypothesis, and the gVisor comparison boundary.
- [`plans/g7-security-checklist.md`](plans/g7-security-checklist.md): proposed
  security reviews, negative tests, fuzzing, and comparative research tasks.
- [`research/g7-security-boundary-assessment.md`](research/g7-security-boundary-assessment.md):
  SEC-01/SEC-02/SEC-11 findings, evidence scope, and claim-specific confidence.
- [`research/guest-image-builds.md`](research/guest-image-builds.md): experimental
  guest-local image builds with automatic builder preparation and an `mk-build` client.
- [`research/docker-in-docker-guests.md`](research/docker-in-docker-guests.md):
  guest-local rootless Docker Engine and ordinary `docker build`/`docker run` verification.
- [`research/docker-online-python-guest.md`](research/docker-online-python-guest.md):
  online Docker Hub/PyPI Python build/run prototype and scoped network evidence.
- [`trial-staging.md`](trial-staging.md): committed-source, protected artifact packets for disposable GCE trials.
- [`plans/README.md`](plans/README.md): ordered implementation and test gates.
- [`../../runtime/README.md`](../../runtime/README.md): future source-tree
  boundaries.
