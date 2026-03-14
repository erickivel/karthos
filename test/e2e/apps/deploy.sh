#!/usr/bin/env bash

set -euo pipefail

VARIANT="${1:-base}"
REGISTRY_PORT="${KIND_REGISTRY_PORT:-5001}"
REGISTRY="localhost:${REGISTRY_PORT}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VARIANT_DIR="${SCRIPT_DIR}/${VARIANT}"

if [[ ! -d "${VARIANT_DIR}" ]]; then
  echo "unknown app variant: ${VARIANT}" >&2
  exit 1
fi

docker build -t "${REGISTRY}/gateway-go:latest" "${VARIANT_DIR}/gateway-go"
docker build -t "${REGISTRY}/service-a-python:latest" "${VARIANT_DIR}/service-a-python"
docker build -t "${REGISTRY}/service-b-node:latest" "${VARIANT_DIR}/service-b-node"

docker push "${REGISTRY}/gateway-go:latest"
docker push "${REGISTRY}/service-a-python:latest"
docker push "${REGISTRY}/service-b-node:latest"

kubectl apply -k "${VARIANT_DIR}"
