#!/usr/bin/env bash
# Tear down Karthos resources on a k3s+ssh cluster.
#
# Default: remove the Karthos Helm release and demo namespaces only.
# With --full: also remove the karthos-registry container on the server.
# Never uninstalls or touches k3s itself.

set -euo pipefail

# shellcheck disable=SC1091
. "$(dirname "${BASH_SOURCE[0]}")/../common.sh"

FULL=0
for arg in "$@"; do
  case "${arg}" in
    --full) FULL=1 ;;
    *) echo "unknown arg: ${arg}" >&2; exit 2 ;;
  esac
done

echo "==> removing Karthos Helm release + demo namespaces"
helm uninstall karthos --namespace karthos-system 2>/dev/null || true
kubectl delete namespace karthos-demo karthos-sparse --ignore-not-found
"${REPO_ROOT}/test/e2e/cluster/istio.sh" reset

if [ "${FULL}" -eq 1 ]; then
  : "${K3S_SSH_HOST:?K3S_SSH_HOST required for --full teardown}"
  echo "==> removing karthos-registry container on ${K3S_SSH_HOST}"
  ssh -o BatchMode=yes "${K3S_SSH_HOST}" "docker rm -f karthos-registry" 2>/dev/null || true
fi

echo "teardown complete (k3s itself was not touched)"
