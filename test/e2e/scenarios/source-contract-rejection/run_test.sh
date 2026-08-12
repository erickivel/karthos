#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-source-contract-rejection-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 && -f "${SOURCE_CONTRACT_TEST_LOG:-}" ]]; then
    cat "${SOURCE_CONTRACT_TEST_LOG}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

cat >"${TMPDIR_TEST}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "kubectl $*" >>"${SOURCE_CONTRACT_TEST_LOG}"

case "${1:-}" in
  get)
    if [[ "${2:-}" == "configmap" ]]; then
      printf '{"data":{"capability-profile.json":"{}","source-plan.json":"{\\"profile_fingerprint\\":\\"valid\\"}"}}'
    elif [[ "${2:-}" == "deployment/oksg-backend" ]]; then
      printf '42'
    elif [[ "${2:-}" == "pods" ]]; then
      printf 'failed-backend'
    fi
    ;;
  replace|annotate|wait)
    ;;
  rollout)
    if [[ "${2:-}" == "restart" ]]; then
      exit 0
    fi
    if [[ "${2:-}" == "status" ]]; then
      count_file="${SOURCE_CONTRACT_TEST_LOG}.rollout-status"
      count=0
      [[ -f "${count_file}" ]] && count="$(cat "${count_file}")"
      count=$((count + 1))
      printf '%s' "${count}" >"${count_file}"
      if [[ "${count}" -eq 1 ]]; then
        exit 1
      fi
    fi
    ;;
  logs)
    printf 'source plan profile fingerprint does not match mounted capability profile\n'
    ;;
esac
EOF
cat >"${TMPDIR_TEST}/task" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "task $*" >>"${SOURCE_CONTRACT_TEST_LOG}"
EOF
chmod +x "${TMPDIR_TEST}/kubectl" "${TMPDIR_TEST}/task"

export PATH="${TMPDIR_TEST}:${PATH}"
export SOURCE_CONTRACT_TEST_LOG="${TMPDIR_TEST}/calls.log"

bash "${REPO_ROOT}/test/e2e/scenarios/source-contract-rejection/run.sh"

restart_line="$(grep -n 'kubectl rollout restart deployment/oksg-backend' "${SOURCE_CONTRACT_TEST_LOG}" | cut -d: -f1)"
wait_line="$(grep -n 'kubectl wait --for=jsonpath=' "${SOURCE_CONTRACT_TEST_LOG}" | head -n1 | cut -d: -f1)"
status_line="$(grep -n 'kubectl rollout status deployment/oksg-backend' "${SOURCE_CONTRACT_TEST_LOG}" | head -n1 | cut -d: -f1)"
[[ -n "${restart_line}" && -n "${wait_line}" && -n "${status_line}" ]]
[[ "${restart_line}" -lt "${wait_line}" && "${wait_line}" -lt "${status_line}" ]]

echo "source-contract rejection waits for the restarted generation: PASS"
