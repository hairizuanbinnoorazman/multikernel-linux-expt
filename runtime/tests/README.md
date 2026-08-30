# Runtime test layers

Tests will be separated into:

- `unit`: no privileges or external services;
- `contract`: fake Kerf, agent, storage, network, and containerd peers;
- `host`: privileged Multikernel tests on a dedicated machine;
- `gce`: billable, explicitly selected cloud tests;
- `containerd`: Runtime v2 behavior;
- `kubernetes`: CRI and `RuntimeClass` behavior;
- `failure`: destructive fault injection using disposable state; and
- `performance`: non-gating until correctness gates pass.

Every privileged suite must perform a preflight inventory and a final resource
inventory even when a test fails.
