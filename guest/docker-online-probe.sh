#!/bin/sh
set -eu
echo ONLINE_DOCKER_PROBE_BEGIN
id
uname -r
grep -E '^(CapEff|CapBnd|NoNewPrivs):' /proc/self/status
/bin/sh /authority-probe.sh
[ ! -e /dev/net/tun ]
echo 'PATH_AUTHORITY path=/dev/net/tun exists=no readable=no writable=no'
docker info --format '{{json .}}'
[ -z "$(docker image ls -q)" ]
echo EMPTY_DAEMON_IMAGE_STORE_CONFIRMED
docker pull python:3.13-slim
echo PYTHON_DOCKERHUB_PULL_PASSED
docker run --rm python:3.13-slim python -c 'import importlib.util; assert importlib.util.find_spec("requests") is None; print("BASE_REQUESTS_ABSENT_CONFIRMED")'
echo PYTHON_BASE_INSPECT_BEGIN
docker image inspect python:3.13-slim
echo PYTHON_BASE_INSPECT_END
docker build -t guest-python-online:latest /online-app
echo ONLINE_PYTHON_BUILD_PASSED
echo PYTHON_APP_RESULT_BEGIN
docker run --rm guest-python-online:latest
echo PYTHON_APP_RESULT_END
echo ONLINE_PYTHON_RUN_PASSED
echo PIP_REPORT_BEGIN
docker run --rm guest-python-online:latest cat /app/pip-report.json
echo PIP_REPORT_END
echo PYTHON_IMAGE_INSPECT_BEGIN
docker image inspect guest-python-online:latest
echo PYTHON_IMAGE_INSPECT_END
echo PYTHON_INNER_AUTHORITY_BEGIN
docker run --rm guest-python-online:latest sh -ec 'id; grep -E "^(CapEff|NoNewPrivs|Seccomp):" /proc/self/status; cat /proc/self/uid_map'
echo PYTHON_INNER_AUTHORITY_END
docker build -t guest-python-online:cached /online-app
echo ONLINE_PYTHON_CACHE_PASSED
docker image rm guest-python-online:latest guest-python-online:cached python:3.13-slim
docker builder prune --all --force
test -z "$(docker ps -aq)"
echo ONLINE_DOCKER_RESOURCES_REMOVED
echo ONLINE_DOCKER_PROBE_END
