#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-deploy-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 && -f "${DEPLOY_TEST_LOG:-}" ]]; then
    cat "${DEPLOY_TEST_LOG}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

export OKSG_IMAGE_TAG_OVERRIDE=run-tag
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"
export DEPLOY_TEST_REGISTRY="${REGISTRY_HOST}"

cat >"${TMPDIR_TEST}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "docker $*" >>"${DEPLOY_TEST_LOG}"
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "curl $*" >>"${DEPLOY_TEST_LOG}"
EOF

cat >"${TMPDIR_TEST}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "kubectl $*" >>"${DEPLOY_TEST_LOG}"

case "${1:-}" in
  kustomize)
    cat <<YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
spec:
  template:
    spec:
      containers:
        - image: ${DEPLOY_TEST_REGISTRY}/gateway-go:latest
        - image: ${DEPLOY_TEST_REGISTRY}/service-a-python:latest
        - image: ${DEPLOY_TEST_REGISTRY}/service-b-node:latest
        - image: ${DEPLOY_TEST_REGISTRY}/mesh-protocol-go:latest
        - image: registry.k8s.io/pause:3.10
YAML
    ;;
  apply)
    [[ "${2:-}" == "-f" ]]
    cp "${3:?missing manifest}" "${DEPLOY_TEST_APPLY_MANIFEST}"
    ;;
  get)
    if [[ "${2:-}" == "nodes" ]]; then
      printf 'test-node\n'
    elif [[ "${2:-}" == "deployments" ]]; then
      printf 'deployment/gateway\n'
    fi
    ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/curl" "${TMPDIR_TEST}/kubectl"

export PATH="${TMPDIR_TEST}:${PATH}"
export DEPLOY_TEST_LOG="${TMPDIR_TEST}/calls.log"
export DEPLOY_TEST_APPLY_MANIFEST="${TMPDIR_TEST}/applied.yaml"

bash "${REPO_ROOT}/test/e2e/apps/deploy.sh" with-istio

for image in gateway-go service-a-python service-b-node mesh-protocol-go; do
  grep -Fqx "        - image: ${DEPLOY_TEST_REGISTRY}/${image}:run-tag" "${DEPLOY_TEST_APPLY_MANIFEST}"
  ! grep -Fq "${DEPLOY_TEST_REGISTRY}/${image}:latest" "${DEPLOY_TEST_APPLY_MANIFEST}"
done
grep -Fqx '        - image: registry.k8s.io/pause:3.10' "${DEPLOY_TEST_APPLY_MANIFEST}"

echo "deploy uses built fixture image tags: PASS"
