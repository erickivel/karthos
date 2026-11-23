#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-deploy-test.XXXXXX)"
TEST_PROFILE="${TMPDIR_TEST}/deploy-contract-test.env"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin"
for command in kind docker go helm kubectl; do
  cat >"${TMPDIR_TEST}/bin/${command}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s %s\n' "$(basename "$0")" "$*" >>"${DEPLOY_TEST_LOG}"
if [[ "$(basename "$0")" == "go" && "${DEPLOY_TEST_FAIL_DISCOVERY:-0}" == "1" && "$*" == *"--mode=discover"* ]]; then
  exit 24
fi
case "$(basename "$0") $*" in
  "kind get clusters") printf 'karthos-test\n' ;;
  "docker inspect "*) printf 'true\n' ;;
  "kubectl get deployment/karthos-backend "*"jsonpath="*) printf 'localhost:5001/karthos-backend:test-tag' ;;
  "kubectl get deployment/karthos-frontend "*"jsonpath="*) printf 'localhost:5001/karthos-frontend:test-tag' ;;
  "kubectl get pods "*"component=backend"*) printf 'localhost:5001/karthos-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' ;;
  "kubectl get pods "*"component=frontend"*) printf 'localhost:5001/karthos-frontend@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' ;;
esac
EOF
  chmod +x "${TMPDIR_TEST}/bin/${command}"
done

cat >"${TMPDIR_TEST}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *karthos-backend*) printf 'Docker-Content-Digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\n' ;;
  *karthos-frontend*) printf 'Docker-Content-Digest: sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\r\n' ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/bin/curl"

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export DEPLOY_TEST_LOG="${TMPDIR_TEST}/calls.log"
export KARTHOS_IMAGE_TAG_OVERRIDE=test-tag
cat >"${TEST_PROFILE}" <<'EOF'
CLUSTER_ENGINE=kind
KIND_CLUSTER_NAME=karthos-test
IMAGE_TAG=latest
EOF
export KARTHOS_CLUSTER_PROFILE_OVERRIDE=deploy-contract-test
export KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE="${TEST_PROFILE}"
export KARTHOS_DEPLOY_IMAGE_BUILDER="${TMPDIR_TEST}/image-builder"
cat >"${KARTHOS_DEPLOY_IMAGE_BUILDER}" <<'EOF'
#!/usr/bin/env bash
printf 'images %s\n' "${IMAGE_TAG}" >>"${DEPLOY_TEST_LOG}"
EOF
chmod +x "${KARTHOS_DEPLOY_IMAGE_BUILDER}"

cd "${REPO_ROOT}"
bash test/e2e/platform/deploy.sh

if grep -F -- '--mode=discover' "${DEPLOY_TEST_LOG}" | grep -Fq -- '--target-namespace'; then
  echo "product deployment must discover Prometheus targets across all namespaces" >&2
  exit 1
fi

grep -Fqx 'images test-tag' "${DEPLOY_TEST_LOG}"
grep -Fq -- '--backend-image-tag=test-tag' "${DEPLOY_TEST_LOG}"
grep -Fq -- '--frontend-image-tag=test-tag' "${DEPLOY_TEST_LOG}"
publish_line="$(grep -n '^images ' "${DEPLOY_TEST_LOG}" | cut -d: -f1)"
helm_line="$(grep -n 'go run ./cmd/discovery-job --mode=deploy' "${DEPLOY_TEST_LOG}" | cut -d: -f1)"
[[ "${publish_line}" -lt "${helm_line}" ]]

: >"${DEPLOY_TEST_LOG}"
cat >"${KARTHOS_DEPLOY_IMAGE_BUILDER}" <<'EOF'
#!/usr/bin/env bash
exit 23
EOF
if bash test/e2e/platform/deploy.sh; then
  echo "deployment should stop when image publication fails" >&2
  exit 1
fi
if grep -Fq -- '--mode=deploy' "${DEPLOY_TEST_LOG}"; then
  echo "Helm deployment ran after image publication failure" >&2
  exit 1
fi

: >"${DEPLOY_TEST_LOG}"
cat >"${KARTHOS_DEPLOY_IMAGE_BUILDER}" <<'EOF'
#!/usr/bin/env bash
printf 'images %s\n' "${IMAGE_TAG}" >>"${DEPLOY_TEST_LOG}"
EOF
export DEPLOY_TEST_FAIL_DISCOVERY=1
if bash test/e2e/platform/deploy.sh; then
  echo "deployment should stop when discovery fails" >&2
  exit 1
fi
unset DEPLOY_TEST_FAIL_DISCOVERY
if grep -Fq -- '--mode=deploy' "${DEPLOY_TEST_LOG}"; then
  echo "Helm deployment ran after discovery failure" >&2
  exit 1
fi

echo "platform deployment shares one tag and stops before Helm on publication failure: PASS"
