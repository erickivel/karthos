#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-undeploy-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin"
cat >"${TMPDIR_TEST}/profile.env" <<'EOF'
CLUSTER_ENGINE=kind
KIND_CLUSTER_NAME=karthos-test
IMAGE_TAG=latest
EOF

cat >"${TMPDIR_TEST}/bin/helm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm %s\n' "$*" >>"${UNDEPLOY_TEST_LOG}"
exit 23
EOF

cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'kubectl %s\n' "$*" >>"${UNDEPLOY_TEST_LOG}"
EOF
chmod +x "${TMPDIR_TEST}/bin/"*

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export UNDEPLOY_TEST_LOG="${TMPDIR_TEST}/calls.log"
export KARTHOS_CLUSTER_PROFILE_OVERRIDE=undeploy-test
export KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE="${TMPDIR_TEST}/profile.env"

if bash "${REPO_ROOT}/test/e2e/platform/undeploy.sh"; then
  echo "undeploy should fail when Helm uninstall fails" >&2
  exit 1
fi
if grep -Fq 'kubectl delete pvc' "${UNDEPLOY_TEST_LOG}"; then
  echo "undeploy deleted PostgreSQL data after Helm uninstall failed" >&2
  exit 1
fi

cat >"${TMPDIR_TEST}/bin/helm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm %s\n' "$*" >>"${UNDEPLOY_TEST_LOG}"
echo 'Error: uninstall: Release not loaded: karthos: release: not found' >&2
exit 1
EOF
: >"${UNDEPLOY_TEST_LOG}"
bash "${REPO_ROOT}/test/e2e/platform/undeploy.sh"
grep -Fq 'kubectl delete pvc data-karthos-postgresql-0' "${UNDEPLOY_TEST_LOG}"

echo "undeploy preserves data on Helm failure and tolerates an absent release: PASS"
