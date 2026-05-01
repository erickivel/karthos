#!/usr/bin/env bash

set -euo pipefail

VARIANT="${1:-base}"
REGISTRY_PORT="${KIND_REGISTRY_PORT:-5001}"
REGISTRY="localhost:${REGISTRY_PORT}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VARIANT_DIR="${SCRIPT_DIR}/${VARIANT}"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

if [[ ! -d "${VARIANT_DIR}" ]]; then
  echo "unknown app variant: ${VARIANT}" >&2
  exit 1
fi

docker build -t "${REGISTRY}/gateway-go:latest" "${VARIANT_DIR}/gateway-go"
docker build -t "${REGISTRY}/service-a-python:latest" "${VARIANT_DIR}/service-a-python"
docker build -t "${REGISTRY}/service-b-node:latest" "${VARIANT_DIR}/service-b-node"
docker build -t "${REGISTRY}/oksg-backend:latest" -f "${REPO_ROOT}/deploy/images/backend/Dockerfile" "${REPO_ROOT}"
docker build -t "${REGISTRY}/oksg-frontend:latest" "${REPO_ROOT}/web"

docker push "${REGISTRY}/gateway-go:latest"
docker push "${REGISTRY}/service-a-python:latest"
docker push "${REGISTRY}/service-b-node:latest"
docker push "${REGISTRY}/oksg-backend:latest"
docker push "${REGISTRY}/oksg-frontend:latest"

kubectl apply -k "${VARIANT_DIR}"
while IFS= read -r deployment; do
  kubectl rollout restart -n oksg-demo "${deployment}"
done < <(kubectl get deployments -n oksg-demo -o name)

while IFS= read -r deployment; do
  kubectl rollout status -n oksg-demo "${deployment}" --timeout=120s
done < <(kubectl get deployments -n oksg-demo -o name)

if kubectl get pod ops-toolbox -n oksg-sparse >/dev/null 2>&1; then
  kubectl wait --for=condition=ready pod/ops-toolbox -n oksg-sparse --timeout=120s
fi
