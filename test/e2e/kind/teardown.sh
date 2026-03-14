#!/usr/bin/env bash

set -euo pipefail

CLUSTER_NAME="${1:-oksg-test}"
REGISTRY_NAME="${KIND_REGISTRY_NAME:-oksg-kind-registry}"

if command -v kind >/dev/null 2>&1; then
  kind delete cluster --name "${CLUSTER_NAME}" || true
fi

if command -v docker >/dev/null 2>&1; then
  docker rm -f "${REGISTRY_NAME}" >/dev/null 2>&1 || true
fi
