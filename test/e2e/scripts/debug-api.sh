#!/usr/bin/env bash

set -euo pipefail

: "${BACKEND_API_LOCAL_PORT:?BACKEND_API_LOCAL_PORT is required}"
: "${KIND_NAMESPACE:?KIND_NAMESPACE is required}"
: "${HELM_RELEASE_NAME:?HELM_RELEASE_NAME is required}"

if lsof -ti ":${BACKEND_API_LOCAL_PORT}" >/dev/null 2>&1; then
  echo "local port ${BACKEND_API_LOCAL_PORT} is busy; choose another with BACKEND_API_LOCAL_PORT=<port> task test:e2e:debug:api" >&2
  exit 1
fi

kubectl port-forward -n "${KIND_NAMESPACE}" "deployment/${HELM_RELEASE_NAME}-backend" "${BACKEND_API_LOCAL_PORT}:8080" &
PF_PID=$!
cleanup_port_forward() {
  kill "${PF_PID}" 2>/dev/null || true
  for _ in $(seq 1 20); do
    kill -0 "${PF_PID}" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL "${PF_PID}" 2>/dev/null || true
  wait "${PF_PID}" 2>/dev/null || true
}
trap cleanup_port_forward EXIT
trap 'exit 130' INT TERM

sleep 2
curl -sf "http://localhost:${BACKEND_API_LOCAL_PORT}/api/v1/status"
