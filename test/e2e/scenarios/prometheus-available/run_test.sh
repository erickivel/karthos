#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-prometheus-run-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin" "${TMPDIR_TEST}/work"
cat >"${TMPDIR_TEST}/bin/task" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'task %s\n' "$*" >>"${PROMETHEUS_RUN_TEST_LOG}"
EOF
cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl %s\n' "$*" >>"${PROMETHEUS_RUN_TEST_LOG}"
if [[ "${1:-}" == "port-forward" ]]; then
  printf '%s\n' "$$" >>"${PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS}"
  trap '/bin/sleep 1; exit 0' TERM
  while true; do /bin/sleep 1; done
fi
EOF
cat >"${TMPDIR_TEST}/bin/python3" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
EOF
cat >"${TMPDIR_TEST}/bin/lsof" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
cat >"${TMPDIR_TEST}/bin/sleep" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "3" ]]; then
  for _ in $(seq 1 100); do
    if [[ -f "${PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS}" ]] && \
      [[ "$(wc -l <"${PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS}")" -eq "${PROMETHEUS_RUN_TEST_EXPECTED_PORT_FORWARDS}" ]]; then
      exit 0
    fi
    /bin/sleep 0.01
  done
  echo "port-forward children did not start before scenario verification" >&2
  exit 1
fi
/bin/sleep 0.1
EOF
chmod +x "${TMPDIR_TEST}/bin/"*

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export PROMETHEUS_RUN_TEST_LOG="${TMPDIR_TEST}/commands.log"
export PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS="${TMPDIR_TEST}/port-forward.pids"
export PROMETHEUS_RUN_TEST_EXPECTED_PORT_FORWARDS=3
export GATEWAY_LOCAL_PORT=18082

cd "${TMPDIR_TEST}/work"
bash "${REPO_ROOT}/test/e2e/scenarios/prometheus-available/run.sh"

grep -Fx 'task test:e2e:scenario:prometheus:prepare' "${PROMETHEUS_RUN_TEST_LOG}"
traffic_calls="$(grep -Fc 'task test:e2e:traffic ' "${PROMETHEUS_RUN_TEST_LOG}" || true)"
[[ "${traffic_calls}" == "2" ]] || { echo "expected two controlled traffic calls" >&2; exit 1; }
traffic_port_overrides="$(grep -Fc 'GATEWAY_LOCAL_PORT=18082' "${PROMETHEUS_RUN_TEST_LOG}" || true)"
[[ "${traffic_port_overrides}" == "2" ]] || { echo "expected runner to pass its gateway port to both traffic calls" >&2; exit 1; }

if [[ ! -s "${PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS}" ]]; then
  echo "Prometheus runner did not start its expected port-forward children" >&2
  cat "${PROMETHEUS_RUN_TEST_LOG}" >&2
  exit 1
fi

while read -r port_forward_pid; do
  if kill -0 "${port_forward_pid}" 2>/dev/null; then
    echo "Prometheus runner returned before its port-forward child exited" >&2
    exit 1
  fi
done <"${PROMETHEUS_RUN_TEST_PORT_FORWARD_PIDS}"

echo "Prometheus runner executes the supported deployment and query flow: PASS"
