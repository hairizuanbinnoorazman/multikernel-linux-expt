# Python app for an online multikernel Docker guest

The approved online guest profile prepares Docker and networking automatically.
Inside that guest, use normal commands:

```sh
docker build -t guest-python-online .
docker run --rm guest-python-online
```

The build uses the official Docker Hub `python:3.13-slim` image and installs
`requests` and its dependencies from `https://pypi.org/simple`, with normal TLS
verification and no package cache. The app fetches public package metadata over
HTTPS and prints the Python/package version and response status as JSON.
The tag is convenient for this example; the evidence records the resolved digest.

This is a diagnostic prototype. See the
[Docker guest design](../../docs/runtime/research/docker-in-docker-guests.md)
for integration and security limits.
