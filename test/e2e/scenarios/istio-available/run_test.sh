#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-istio-run-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 && -f "${ISTIO_RUN_TEST_APPLY_MANIFEST:-}" ]]; then
    cat "${ISTIO_RUN_TEST_APPLY_MANIFEST}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

export OKSG_IMAGE_TAG_OVERRIDE=run-tag
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"
export ISTIO_RUN_TEST_REGISTRY="${REGISTRY_HOST}"

cat >"${TMPDIR_TEST}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  port-forward)
    printf '%s\n' "$$" >>"${ISTIO_RUN_TEST_PORT_FORWARD_PIDS}"
    trap '/bin/sleep 1; exit 0' TERM
    while true; do /bin/sleep 1; done
    ;;
  apply)
    [[ "${2:-}" == "-f" ]]
    cp "${3:?missing manifest}" "${ISTIO_RUN_TEST_APPLY_MANIFEST}"
    ;;
  logs)
    printf 'WARMUP_TRAFFIC_COMPLETE\nMEASURED_TRAFFIC_COMPLETE\n'
    ;;
  get)
    if [[ "${2:-}" == "pods" ]]; then
      printf 'mesh-protocol-server-0\n'
    fi
    ;;
esac
EOF

cat >"${TMPDIR_TEST}/task" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${ISTIO_RUN_TEST_TASK_LOG}"
EOF

cat >"${TMPDIR_TEST}/python3" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF

cat >"${TMPDIR_TEST}/lsof" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exit 1
EOF

cat >"${TMPDIR_TEST}/sleep" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "3" ]]; then
  for _ in $(seq 1 100); do
    if [[ -f "${ISTIO_RUN_TEST_PORT_FORWARD_PIDS}" ]] && \
      [[ "$(wc -l <"${ISTIO_RUN_TEST_PORT_FORWARD_PIDS}")" -eq "${ISTIO_RUN_TEST_EXPECTED_PORT_FORWARDS}" ]]; then
      exit 0
    fi
    /bin/sleep 0.01
  done
  echo "port-forward children did not start before scenario verification" >&2
  exit 1
fi
/bin/sleep 0.1
EOF
chmod +x "${TMPDIR_TEST}/kubectl" "${TMPDIR_TEST}/task" "${TMPDIR_TEST}/python3" \
  "${TMPDIR_TEST}/lsof" "${TMPDIR_TEST}/sleep"

export PATH="${TMPDIR_TEST}:${PATH}"
export ISTIO_RUN_TEST_APPLY_MANIFEST="${TMPDIR_TEST}/applied.yaml"
export ISTIO_RUN_TEST_TASK_LOG="${TMPDIR_TEST}/task.log"
export ISTIO_RUN_TEST_PORT_FORWARD_PIDS="${TMPDIR_TEST}/port-forward.pids"
export ISTIO_RUN_TEST_EXPECTED_PORT_FORWARDS=4

cd "${TMPDIR_TEST}"
GATEWAY_LOCAL_PORT=18082 bash "${REPO_ROOT}/test/e2e/scenarios/istio-available/run.sh"

grep -Fqx "          image: ${ISTIO_RUN_TEST_REGISTRY}/mesh-protocol-go:run-tag" "${ISTIO_RUN_TEST_APPLY_MANIFEST}"
grep -Fqx "test:e2e:scenario:istio:prepare GATEWAY_LOCAL_PORT=18082" "${ISTIO_RUN_TEST_TASK_LOG}"

if [[ ! -s "${ISTIO_RUN_TEST_PORT_FORWARD_PIDS}" ]]; then
  echo "Istio runner did not start its expected port-forward children" >&2
  exit 1
fi

while read -r port_forward_pid; do
  if kill -0 "${port_forward_pid}" 2>/dev/null; then
    echo "Istio runner returned before its port-forward child exited" >&2
    exit 1
  fi
done <"${ISTIO_RUN_TEST_PORT_FORWARD_PIDS}"

echo "Istio protocol Job uses built fixture image tag: PASS"
