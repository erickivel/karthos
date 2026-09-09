#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

VARIANT="${1:-base}"
VARIANT_DIR="${SCRIPT_DIR}/${VARIANT}"
APP_SOURCE_DIR="${VARIANT_DIR}"
if [[ "${VARIANT}" == "with-prometheus" || "${VARIANT}" == "with-istio" ]]; then
  APP_SOURCE_DIR="${SCRIPT_DIR}/instrumented"
fi
IMAGE_LOAD="${REPO_ROOT}/test/e2e/cluster/image.sh"
RENDERED_DIR="$(mktemp -d -t karthos-e2e-apps.XXXXXX)"
trap 'rm -rf "${RENDERED_DIR}"' EXIT

if [[ ! -d "${VARIANT_DIR}" ]]; then
  echo "unknown app variant: ${VARIANT}" >&2
  exit 1
fi

echo "deploying e2e app variant: ${VARIANT}"

build_and_load() {
  local tag="$1"
  local context="$2"
  local dockerfile="${3:-}"
  local platform_args=()
  if [[ -n "${IMAGE_PLATFORM:-}" ]]; then
    platform_args=(--platform "${IMAGE_PLATFORM}")
  fi
  # `"${arr[@]+"${arr[@]}"}"` expands to nothing when the array is empty
  # and to its elements otherwise. Required because bash 3.2 (macOS system
  # bash) under `set -u` treats `"${arr[@]}"` on an empty array as unbound.
  if [[ -n "${dockerfile}" ]]; then
    docker build "${platform_args[@]+"${platform_args[@]}"}" -t "${tag}" -f "${dockerfile}" "${context}"
  else
    docker build "${platform_args[@]+"${platform_args[@]}"}" -t "${tag}" "${context}"
  fi
  "${IMAGE_LOAD}" push_or_load "${tag}"
}

build_and_load "${REGISTRY_HOST}/gateway-go:${IMAGE_TAG}" "${APP_SOURCE_DIR}/gateway-go"
build_and_load "${REGISTRY_HOST}/service-a-python:${IMAGE_TAG}" "${APP_SOURCE_DIR}/service-a-python"
build_and_load "${REGISTRY_HOST}/service-b-node:${IMAGE_TAG}" "${APP_SOURCE_DIR}/service-b-node"
if [[ "${VARIANT}" == "with-istio" ]]; then
  build_and_load "${REGISTRY_HOST}/mesh-protocol-go:${IMAGE_TAG}" "${SCRIPT_DIR}/mesh-protocol-go"
fi

RENDERED_APPS="${RENDERED_DIR}/apps.yaml"
kubectl kustomize "${VARIANT_DIR}" >"${RENDERED_APPS}"
REGISTRY_HOST_PATTERN="${REGISTRY_HOST//./\\.}"
sed -i.bak \
  -e "s|${REGISTRY_HOST_PATTERN}/gateway-go:latest|${REGISTRY_HOST}/gateway-go:${IMAGE_TAG}|g" \
  -e "s|${REGISTRY_HOST_PATTERN}/service-a-python:latest|${REGISTRY_HOST}/service-a-python:${IMAGE_TAG}|g" \
  -e "s|${REGISTRY_HOST_PATTERN}/service-b-node:latest|${REGISTRY_HOST}/service-b-node:${IMAGE_TAG}|g" \
  -e "s|${REGISTRY_HOST_PATTERN}/mesh-protocol-go:latest|${REGISTRY_HOST}/mesh-protocol-go:${IMAGE_TAG}|g" \
  "${RENDERED_APPS}"
rm -f "${RENDERED_APPS}.bak"

if [[ "${CLUSTER_ENGINE}" == "k3s" ]]; then
  REMOTE_NODE_NAME="${K3S_NODE_NAME:-$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')}"
  if [[ -z "${REMOTE_NODE_NAME}" ]]; then
    echo "could not resolve remote k3s node name for app placement" >&2
    exit 1
  fi
  echo "remote k3s profile: mapping kind fixture node selectors to ${REMOTE_NODE_NAME}"
  sed -i.bak \
    -e "s/karthos-test-control-plane/${REMOTE_NODE_NAME}/g" \
    -e "s/karthos-test-worker/${REMOTE_NODE_NAME}/g" \
    "${RENDERED_APPS}"
  rm -f "${RENDERED_APPS}.bak"
  kubectl delete pod ops-toolbox -n karthos-sparse --ignore-not-found
fi

kubectl apply -f "${RENDERED_APPS}"
active_deployments=()
while IFS= read -r deployment; do
  replicas="$(kubectl get "${deployment}" -n karthos-demo -o jsonpath='{.spec.replicas}')"
  if [[ "${replicas:-1}" == "0" ]]; then
    continue
  fi
  active_deployments+=("${deployment}")
  kubectl rollout restart -n karthos-demo "${deployment}"
done < <(kubectl get deployments -n karthos-demo -o name)

for deployment in "${active_deployments[@]}"; do
  kubectl rollout status -n karthos-demo "${deployment}" --timeout=120s
done

if kubectl get pod ops-toolbox -n karthos-sparse >/dev/null 2>&1; then
  kubectl wait --for=condition=ready pod/ops-toolbox -n karthos-sparse --timeout=120s
fi
