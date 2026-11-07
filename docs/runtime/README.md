# Runtime design documentation

- [`status.md`](status.md): proved results, current client feature matrix, and
  safety boundary.
- [Archived checkpoint notes](history/readme-checkpoints-2026-11-03.md): the
  topic-grouped progress narrative moved out of the root README; historical
  statements retain their original source and scope.
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
- [`contracts/runtime-restart-checkpoint.md`](contracts/runtime-restart-checkpoint.md): normal private-runtime restart, same-child Docker rebuild continuity and exact primary image absence.
- [`contracts/synced-write-runtime-crash.md`](contracts/synced-write-runtime-crash.md): synced child writes during confirmed runtime outage/adoption, stamped data readback and native byte coverage.
- [`contracts/concurrent-child-builds.md`](contracts/concurrent-child-builds.md): two private engines/backings, real RUN overlap, sibling removal and survivor rebuild.
- [`plans/README.md`](plans/README.md): ordered implementation and test gates.
- [`../../runtime/README.md`](../../runtime/README.md): future source-tree
  boundaries.
