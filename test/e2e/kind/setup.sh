#!/usr/bin/env bash

set -euo pipefail

CLUSTER_NAME="${1:-oksg-test}"
REGISTRY_NAME="${KIND_REGISTRY_NAME:-oksg-kind-registry}"
REGISTRY_PORT="${KIND_REGISTRY_PORT:-5001}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/cluster-config.yaml"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

if ! command -v kind >/dev/null 2>&1; then
  echo "kind is required" >&2
  exit 1
fi

if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is required" >&2
  exit 1
fi

running_registry="$(docker inspect -f '{{.State.Running}}' "${REGISTRY_NAME}" 2>/dev/null || true)"
if [[ "${running_registry}" != "true" ]]; then
  docker run -d --restart=always -p "127.0.0.1:${REGISTRY_PORT}:5000" --name "${REGISTRY_NAME}" registry:2
fi

if ! kind get clusters | rg -x "${CLUSTER_NAME}" >/dev/null 2>&1; then
  kind create cluster --name "${CLUSTER_NAME}" --config "${CONFIG_FILE}"
fi

if [[ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${REGISTRY_NAME}")" == "null" ]]; then
  docker network connect "kind" "${REGISTRY_NAME}"
fi

cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

kubectl wait --namespace kube-system --for=condition=Ready pods --all --timeout=120s
