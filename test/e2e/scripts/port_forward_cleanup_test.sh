#!/usr/bin/env bash

set -euo pipefail

TMPDIR_TEST="$(mktemp -d -t karthos-port-forward-cleanup-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ -f "${TMPDIR_TEST}/port-forward.pid" ]]; then
    kill "$(cat "${TMPDIR_TEST}/port-forward.pid")" 2>/dev/null || true
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin"
cat >"${TMPDIR_TEST}/bin/lsof" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "port-forward" ]]; then
  exit 1
fi
printf '%s' "$$" >"${PORT_FORWARD_TEST_DIR}/port-forward.pid"
trap 'sleep 1; exit 0' TERM
while true; do sleep 1; done
EOF
cat >"${TMPDIR_TEST}/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf '{"service":"gateway-go","results":[]}'
EOF
cat >"${TMPDIR_TEST}/bin/jq" <<'EOF'
#!/usr/bin/env bash
cat >/dev/null
printf '{}\n'
EOF
chmod +x "${TMPDIR_TEST}/bin/"*

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export PORT_FORWARD_TEST_DIR="${TMPDIR_TEST}"

task test:e2e:traffic GATEWAY_LOCAL_PORT=18083 TRAFFIC_REQUESTS=1 TRAFFIC_INTERVAL=0

port_forward_pid="$(cat "${TMPDIR_TEST}/port-forward.pid")"
if kill -0 "${port_forward_pid}" 2>/dev/null; then
  echo "e2e:traffic returned before its port-forward child exited" >&2
  exit 1
fi

BACKEND_API_LOCAL_PORT=18084 KIND_NAMESPACE=karthos-system HELM_RELEASE_NAME=karthos ./test/e2e/scripts/debug-api.sh

debug_port_forward_pid="$(cat "${TMPDIR_TEST}/port-forward.pid")"
if kill -0 "${debug_port_forward_pid}" 2>/dev/null; then
  echo "debug-api returned before its port-forward child exited" >&2
  exit 1
fi

echo "traffic and debug helpers wait for their owned port-forward processes: PASS"
