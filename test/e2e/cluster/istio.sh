#!/usr/bin/env bash

set -euo pipefail

ISTIO_NAMESPACE="istio-system"
OWNERSHIP_MARKER="karthos-e2e-istio-owner"

usage() {
  echo "usage: istio.sh <install|reset>" >&2
  exit 2
}

namespace_exists() {
  kubectl get namespace "${ISTIO_NAMESPACE}" >/dev/null 2>&1
}

harness_owns_istio() {
  kubectl get configmap "${OWNERSHIP_MARKER}" -n "${ISTIO_NAMESPACE}" >/dev/null 2>&1
}

install() {
  if namespace_exists; then
    if harness_owns_istio; then
      echo "Istio is already installed by this E2E harness"
    else
      echo "Istio already exists and is not E2E-owned; using the existing control plane"
    fi
    return
  fi

  istioctl install --set profile=minimal -y
  kubectl create configmap "${OWNERSHIP_MARKER}" -n "${ISTIO_NAMESPACE}" \
    --from-literal=owner=karthos-e2e \
    --from-literal=purpose=live-e2e
}

reset() {
  if ! namespace_exists; then
    return
  fi
  if ! harness_owns_istio; then
    echo "Istio exists but is not owned by the Karthos E2E harness. Remove it manually or use a clean cluster before running strict fallback." >&2
    return 1
  fi

  istioctl uninstall --purge -y
  kubectl delete namespace "${ISTIO_NAMESPACE}" --ignore-not-found
  kubectl wait --for=delete "namespace/${ISTIO_NAMESPACE}" --timeout=180s
}

case "${1:-}" in
  install) install ;;
  reset) reset ;;
  *) usage ;;
esac
