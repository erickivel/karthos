#!/usr/bin/env bash

set -euo pipefail

case "${CLUSTER_ENGINE}" in
  kind)
    if ! kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER_NAME:-karthos-test}"; then
      echo "Cluster not found. Run 'task test:e2e:setup' first." >&2
      exit 1
    fi
    if ! docker inspect -f '{{.State.Running}}' karthos-kind-registry 2>/dev/null | grep -q true; then
      echo "Registry not running. Run 'task test:e2e:setup' first." >&2
      exit 1
    fi
    ;;
  k3s)
    if ! kubectl --kubeconfig "${KUBECONFIG}" get nodes >/dev/null 2>&1; then
      echo "kubectl cannot reach the cluster (KUBECONFIG=${KUBECONFIG}). Run 'task test:e2e:setup' and 'task test:cluster:doctor'." >&2
      exit 1
    fi
    if ! ssh -o BatchMode=yes -o ConnectTimeout=5 "${K3S_SSH_HOST}" curl -fsS --max-time 5 http://127.0.0.1:5001/v2/ >/dev/null 2>&1; then
      echo "remote registry is not reachable from the k3s server at 127.0.0.1:5001. Run 'task test:e2e:setup' and 'task test:cluster:doctor'." >&2
      exit 1
    fi
    ;;
  *)
    echo "unsupported CLUSTER_ENGINE: ${CLUSTER_ENGINE}" >&2
    exit 1
    ;;
esac
