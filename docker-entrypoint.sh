#!/bin/sh
set -e

DATA_DIR="${DATA_DIR:-/app/data}"
DOCKER_SOCKET="${DOCKER_SOCKET:-/var/run/docker.sock}"

mkdir -p "$DATA_DIR"

if [ ! -S "$DOCKER_SOCKET" ]; then
  echo "warning: $DOCKER_SOCKET is not mounted; docker actions will fail" >&2
fi

# Root is required to edit host compose files (typically root:root) and to talk
# to the engine. Docker socket access is already equivalent to root on the host.
exec /app/ops-console
