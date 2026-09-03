#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-reset-all-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin"
cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1 $2" == "get namespace" ]]; then
  exit 0
fi
if [[ "$1 $2" == "get configmap" ]]; then
  [[ "${RESET_ALL_ISTIO_STATE}" == "owned" ]]
  exit
fi
printf 'kubectl %s\n' "$*" >>"${RESET_ALL_ACTION_LOG}"
EOF
cat >"${TMPDIR_TEST}/bin/helm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm %s\n' "$*" >>"${RESET_ALL_ACTION_LOG}"
EOF
cat >"${TMPDIR_TEST}/bin/istioctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'istioctl %s\n' "$*" >>"${RESET_ALL_ACTION_LOG}"
EOF
chmod +x "${TMPDIR_TEST}/bin/kubectl" "${TMPDIR_TEST}/bin/helm" "${TMPDIR_TEST}/bin/istioctl"

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export RESET_ALL_ACTION_LOG="${TMPDIR_TEST}/actions.log"

cd "${REPO_ROOT}"
if RESET_ALL_ISTIO_STATE=external task test:e2e:reset-all >"${TMPDIR_TEST}/reset-all.out" 2>&1; then
  echo "strict fallback should reject externally owned Istio" >&2
  exit 1
fi

if ! grep -Fq 'not owned by the OKSG E2E harness' "${TMPDIR_TEST}/reset-all.out"; then
  cat "${TMPDIR_TEST}/reset-all.out" >&2
  exit 1
fi
if [[ -s "${RESET_ALL_ACTION_LOG}" ]]; then
  echo "strict fallback changed E2E state before rejecting externally owned Istio:" >&2
  cat "${RESET_ALL_ACTION_LOG}" >&2
  exit 1
fi

: >"${RESET_ALL_ACTION_LOG}"
RESET_ALL_ISTIO_STATE=owned task test:e2e:reset-all

first_action="$(head -n1 "${RESET_ALL_ACTION_LOG}")"
[[ "${first_action}" == "istioctl uninstall --purge -y" ]] || {
  echo "E2E-owned Istio was not reset before other cleanup: ${first_action}" >&2
  exit 1
}
grep -Fx 'kubectl delete namespace istio-system --ignore-not-found' "${RESET_ALL_ACTION_LOG}"
grep -Fx 'helm uninstall oksg-otel --namespace oksg-system' "${RESET_ALL_ACTION_LOG}"
grep -Fx 'kubectl delete namespace oksg-demo --ignore-not-found' "${RESET_ALL_ACTION_LOG}"

echo "strict fallback rejects external Istio and resets E2E-owned Istio first: PASS"
