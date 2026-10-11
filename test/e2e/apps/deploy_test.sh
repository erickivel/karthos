#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-deploy-test.XXXXXX)"
TEST_PROFILE="${TMPDIR_TEST}/deploy-test.env"
cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 && -f "${DEPLOY_TEST_LOG:-}" ]]; then
    cat "${DEPLOY_TEST_LOG}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

export KARTHOS_IMAGE_TAG_OVERRIDE=run-tag
cat >"${TEST_PROFILE}" <<'EOF'
CLUSTER_ENGINE=k3s
K3S_SSH_HOST=fixture-host
IMAGE_TAG=latest
EOF
export KARTHOS_CLUSTER_PROFILE_OVERRIDE=deploy-test
export KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE="${TEST_PROFILE}"
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"
export DEPLOY_TEST_REGISTRY="${REGISTRY_HOST}"

cat >"${TMPDIR_TEST}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${DEPLOY_TEST_REMOTE:-0}" == "1" ]]; then
  echo "remote docker $*" >>"${DEPLOY_TEST_LOG}"
  case "$1" in
    load)
      [[ "$(cat)" == "image-stream" ]] || exit 1
      printf 'Loaded image\n'
      ;;
    push)
      [[ "${2:-}" == "${DEPLOY_TEST_REGISTRY}"/*:run-tag ]] || exit 1
      printf 'digest: sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc size: 1\n'
      ;;
    *)
      exit 1
      ;;
  esac
  exit 0
fi
echo "docker $*" >>"${DEPLOY_TEST_LOG}"
if [[ "${1:-}" == "build" && ( "$*" == *"/deploy/images/backend/Dockerfile"* || "$*" == *"/web"* ) ]]; then
  echo "fixture deployer must not build platform images" >&2
  exit 1
fi
if [[ "${1:-}" == "save" ]]; then
  printf 'image-stream\n'
fi
EOF

cat >"${TMPDIR_TEST}/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "ssh $*" >>"${DEPLOY_TEST_LOG}"
remote_command=""
for argument in "$@"; do
  remote_command="${argument}"
done
[[ "${remote_command}" == "bash -c "* ]] || exit 1
[[ "${remote_command}" == *"set -euo pipefail"* ]] || exit 1
DEPLOY_TEST_REMOTE=1 bash -c "${remote_command}"
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "curl $*" >>"${DEPLOY_TEST_LOG}"
[[ "${DEPLOY_TEST_REMOTE:-0}" == "1" ]] || exit 1
printf 'Docker-Content-Digest: sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\r\n'
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
      printf 'deployment/gateway\ndeployment/config-bundle\n'
    elif [[ "${2:-}" == "deployment/gateway" ]]; then
      printf '1'
    elif [[ "${2:-}" == "deployment/config-bundle" ]]; then
      printf '0'
    fi
    ;;
  rollout)
    ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/ssh" "${TMPDIR_TEST}/curl" "${TMPDIR_TEST}/kubectl"

export PATH="${TMPDIR_TEST}:${PATH}"
export DEPLOY_TEST_LOG="${TMPDIR_TEST}/calls.log"
export DEPLOY_TEST_APPLY_MANIFEST="${TMPDIR_TEST}/applied.yaml"

bash "${REPO_ROOT}/test/e2e/apps/deploy.sh" with-istio

expected_images=(gateway-go service-a-python service-b-node mesh-protocol-go)
for image in "${expected_images[@]}"; do
  grep -Fqx "        - image: ${DEPLOY_TEST_REGISTRY}/${image}:run-tag" "${DEPLOY_TEST_APPLY_MANIFEST}"
  grep -Fqx "remote docker push ${DEPLOY_TEST_REGISTRY}/${image}:run-tag" "${DEPLOY_TEST_LOG}"
done
grep -Fqx '        - image: registry.k8s.io/pause:3.10' "${DEPLOY_TEST_APPLY_MANIFEST}"
grep -Fqx 'kubectl rollout restart -n karthos-demo deployment/gateway' "${DEPLOY_TEST_LOG}"
actual_pushes="$(grep '^remote docker push ' "${DEPLOY_TEST_LOG}" | sort)"
expected_pushes="$(for image in "${expected_images[@]}"; do echo "remote docker push ${DEPLOY_TEST_REGISTRY}/${image}:run-tag"; done | sort)"
[[ "${actual_pushes}" == "${expected_pushes}" ]]
actual_builds="$(grep '^docker build ' "${DEPLOY_TEST_LOG}" | sed -E 's/.* -t [^ ]+ //')"
expected_builds="$(printf '%s\n' \
  "${REPO_ROOT}/test/e2e/apps/instrumented/gateway-go" \
  "${REPO_ROOT}/test/e2e/apps/instrumented/service-a-python" \
  "${REPO_ROOT}/test/e2e/apps/instrumented/service-b-node" \
  "${REPO_ROOT}/test/e2e/apps/mesh-protocol-go")"
[[ "${actual_builds}" == "${expected_builds}" ]]
actual_restarts="$(grep '^kubectl rollout restart ' "${DEPLOY_TEST_LOG}" || true)"
[[ "${actual_restarts}" == 'kubectl rollout restart -n karthos-demo deployment/gateway' ]]

echo "deploy uses built fixture image tags: PASS"
