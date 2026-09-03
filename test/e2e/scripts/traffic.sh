#!/usr/bin/env bash

set -euo pipefail

MODE="${1:-}"
case "${MODE}" in
  once|watch) ;;
  *) echo "usage: traffic.sh <once|watch>" >&2; exit 2 ;;
esac

: "${GATEWAY_LOCAL_PORT:?GATEWAY_LOCAL_PORT is required}"
: "${TARGET_NAMESPACE:?TARGET_NAMESPACE is required}"
: "${TRAFFIC_INTERVAL:?TRAFFIC_INTERVAL is required}"

if lsof -ti ":${GATEWAY_LOCAL_PORT}" >/dev/null 2>&1; then
  echo "local port ${GATEWAY_LOCAL_PORT} is busy; choose another with GATEWAY_LOCAL_PORT=<port> task test:e2e:traffic" >&2
  exit 1
fi

kubectl port-forward -n "${TARGET_NAMESPACE}" svc/gateway-go "${GATEWAY_LOCAL_PORT}:8080" &
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
send_normal() {
  echo -n "  [ok $1] "
  curl -sf "http://localhost:${GATEWAY_LOCAL_PORT}/" | jq -c '{gateway: .service, results: [.results[] | {s: .service, status: .status}]}' 2>/dev/null || echo "(request failed)"
}
send_error() {
  echo -n "  [error $1] "
  curl -s "http://localhost:${GATEWAY_LOCAL_PORT}/error" | jq -c '{gateway: .service, mode: .mode, results: [.results[] | {s: .service, status: .status}]}' 2>/dev/null || echo "(request failed)"
}

if [[ "${MODE}" == "watch" ]]; then
  : "${TRAFFIC_ERROR_EVERY:?TRAFFIC_ERROR_EVERY is required for watch}"
  echo "Streaming requests to gateway (Ctrl-C to stop)..."
  i=0
  while true; do
    i=$((i + 1))
    if [[ "${TRAFFIC_ERROR_EVERY}" -gt 0 && $((i % TRAFFIC_ERROR_EVERY)) -eq 0 ]]; then
      send_error "${i}"
    else
      send_normal "${i}"
    fi
    sleep "${TRAFFIC_INTERVAL}"
  done
fi

: "${TRAFFIC_REQUESTS:?TRAFFIC_REQUESTS is required for once}"
: "${TRAFFIC_ERROR_REQUESTS:?TRAFFIC_ERROR_REQUESTS is required for once}"
: "${TRAFFIC_INTERLEAVE_ERRORS:?TRAFFIC_INTERLEAVE_ERRORS is required for once}"

if [[ "${TRAFFIC_INTERLEAVE_ERRORS}" == "true" && "${TRAFFIC_ERROR_REQUESTS}" -gt 0 ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  # shellcheck disable=SC1091
  . "${SCRIPT_DIR}/traffic_schedule.sh"
  total=$((TRAFFIC_REQUESTS + TRAFFIC_ERROR_REQUESTS))
  echo "Sending ${total} interleaved requests (${TRAFFIC_REQUESTS} normal, ${TRAFFIC_ERROR_REQUESTS} errors)..."
  normal_i=0
  error_i=0
  for request_kind in $(interleaved_request_kinds "${TRAFFIC_REQUESTS}" "${TRAFFIC_ERROR_REQUESTS}"); do
    if [[ "${request_kind}" == "error" ]]; then
      error_i=$((error_i + 1))
      send_error "${error_i}/${TRAFFIC_ERROR_REQUESTS}"
    else
      normal_i=$((normal_i + 1))
      send_normal "${normal_i}/${TRAFFIC_REQUESTS}"
    fi
    sleep "${TRAFFIC_INTERVAL}"
  done
else
  echo "Sending ${TRAFFIC_REQUESTS} requests to gateway (triggers service-a + service-b calls)..."
  for i in $(seq 1 "${TRAFFIC_REQUESTS}"); do
    send_normal "${i}/${TRAFFIC_REQUESTS}"
    sleep "${TRAFFIC_INTERVAL}"
  done
  if [[ "${TRAFFIC_ERROR_REQUESTS}" -gt 0 ]]; then
    echo "Sending ${TRAFFIC_ERROR_REQUESTS} intentional error requests to gateway /error..."
    for i in $(seq 1 "${TRAFFIC_ERROR_REQUESTS}"); do
      send_error "${i}/${TRAFFIC_ERROR_REQUESTS}"
      sleep "${TRAFFIC_INTERVAL}"
    done
  fi
fi

echo "Done. Run 'task test:e2e:debug:api' to inspect backend status and query /api/v1/graph or /api/v1/metrics for data."
