#!/usr/bin/env bash
# Per-cell image transport.
#
# Today this is a thin wrapper around `docker push` for both supported
# cells — the only `case` arm is `push_or_load`, which always pushes:
#   - kind+local: docker push -> local "registry:2" container that kind's
#     containerd mirrors at localhost:5001.
#   - k3s+ssh:   docker push -> SSH tunnel -> remote "registry:2" container.
#     k3s containerd mirrors localhost:5001 -> http://127.0.0.1:5001 via
#     /etc/rancher/k3s/registries.yaml.
#
# The wrapper exists so deploy.sh never needs to know which transport a
# cell uses; a future kind variant can swap in `kind load docker-image`
# here (paired with flipping pullPolicy=Always) without touching deploy.sh.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"

usage() {
  cat >&2 <<EOF
usage: image.sh push_or_load <fully-qualified-tag>
example: image.sh push_or_load localhost:5001/oksg-backend:latest
EOF
  exit 2
}

case "${1:-}" in
  push_or_load)
    shift
    [ $# -eq 1 ] || usage
    docker push "$1"
    ;;
  *)
    usage
    ;;
esac
