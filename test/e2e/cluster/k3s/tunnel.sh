#!/usr/bin/env bash
# Foreground SSH tunnel that makes localhost:${K3S_REGISTRY_PORT} on the dev
# machine forward to localhost:${K3S_REGISTRY_PORT} on the k3s server.
#
# This is what makes `docker push localhost:5001/...` reach the registry
# container on the remote host. Ctrl-C to stop. Keep it in its own pane.

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
