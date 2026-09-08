#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-istio-owner-test.XXXXXX)"
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
printf 'kubectl %s\n' "$*" >>"${ISTIO_TEST_LOG}"
if [[ "$1 $2" == "get namespace" ]]; then
  [[ "${ISTIO_TEST_STATE}" != "missing" ]]
  exit
fi
if [[ "$1 $2" == "get configmap" ]]; then
  [[ "${ISTIO_TEST_STATE}" == "owned" ]]
  exit
fi
EOF
cat >"${TMPDIR_TEST}/bin/istioctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'istioctl %s\n' "$*" >>"${ISTIO_TEST_LOG}"
EOF
chmod +x "${TMPDIR_TEST}/bin/kubectl" "${TMPDIR_TEST}/bin/istioctl"

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export ISTIO_TEST_LOG="${TMPDIR_TEST}/commands.log"

ISTIO_TEST_STATE=missing bash "${REPO_ROOT}/test/e2e/cluster/istio.sh" install
grep -Fx 'istioctl install --set profile=minimal -y' "${ISTIO_TEST_LOG}"
grep -Fx 'kubectl create configmap oksg-e2e-istio-owner -n istio-system --from-literal=owner=oksg-e2e --from-literal=purpose=live-e2e' "${ISTIO_TEST_LOG}"

: >"${ISTIO_TEST_LOG}"
ISTIO_TEST_STATE=owned bash "${REPO_ROOT}/test/e2e/cluster/istio.sh" reset
grep -Fx 'istioctl uninstall --purge -y' "${ISTIO_TEST_LOG}"
grep -Fx 'kubectl delete namespace istio-system --ignore-not-found' "${ISTIO_TEST_LOG}"
grep -Fx 'kubectl wait --for=delete namespace/istio-system --timeout=180s' "${ISTIO_TEST_LOG}"

: >"${ISTIO_TEST_LOG}"
if ISTIO_TEST_STATE=external bash "${REPO_ROOT}/test/e2e/cluster/istio.sh" reset >"${TMPDIR_TEST}/external.out" 2>&1; then
  echo "external Istio reset should fail" >&2
  exit 1
fi
grep -Fq 'not owned by the OKSG E2E harness' "${TMPDIR_TEST}/external.out"
[[ ! -s "${ISTIO_TEST_LOG}" ]]

echo "Istio ownership cleanup is strict and safe: PASS"
