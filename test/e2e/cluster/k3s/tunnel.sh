#!/usr/bin/env bash
# Foreground SSH tunnel that makes localhost:5001 on the development machine
# forward to localhost:5001 on the k3s server.
#
# E2E image deploys stream images to the remote Docker daemon directly, so this
# tunnel is optional and only useful for inspecting the remote registry locally.
# Ctrl-C to stop. Keep it in its own pane when using it.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/../common.sh"

: "${K3S_SSH_HOST:?K3S_SSH_HOST required (check .env.d/${CLUSTER_PROFILE}.env)}"
echo "tunnelling localhost:5001 -> ${K3S_SSH_HOST}:5001"
echo "(Ctrl-C to stop)"
exec ssh -N \
  -o ExitOnForwardFailure=yes \
  -o ServerAliveInterval=30 \
  -L "5001:localhost:5001" \
  "${K3S_SSH_HOST}"
