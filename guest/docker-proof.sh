#!/bin/sh

echo "DOCKER_IMAGE_READY"
echo "release=$(uname -r)"
echo "pid1=$(readlink /proc/1/exe)"
echo "self=$(readlink /proc/self/exe)"
grep ' / daxfs ' /proc/mounts
cat /etc/os-release 2>/dev/null || true
while true; do sleep 3600; done
