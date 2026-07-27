#!/usr/bin/env bash
# Foreground SSH tunnel that makes localhost:${K3S_REGISTRY_PORT} on the dev
# machine forward to localhost:${K3S_REGISTRY_PORT} on the k3s server.
#
# E2E image deploys stream images to the remote Docker daemon directly, so this
# tunnel is optional and only useful for inspecting the remote registry locally.
# Ctrl-C to stop. Keep it in its own pane when using it.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/../common.sh"

: "${K3S_SSH_HOST:?K3S_SSH_HOST required (check .env.d/${CLUSTER_PROFILE}.env)}"
: "${K3S_REGISTRY_PORT:=5001}"

echo "tunnelling localhost:${K3S_REGISTRY_PORT} -> ${K3S_SSH_HOST}:${K3S_REGISTRY_PORT}"
echo "(Ctrl-C to stop)"
exec ssh -N \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -L "${K3S_REGISTRY_PORT}:localhost:${K3S_REGISTRY_PORT}" \
  "${K3S_SSH_HOST}"
