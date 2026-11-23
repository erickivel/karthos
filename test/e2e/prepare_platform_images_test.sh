#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-platform-images-test.XXXXXX)"
TEST_PROFILE="${TMPDIR_TEST}/platform-images-test.env"
cleanup() {
  local status=$?
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
export KARTHOS_CLUSTER_PROFILE_OVERRIDE=platform-images-test
export KARTHOS_CLUSTER_PROFILE_FILE_OVERRIDE="${TEST_PROFILE}"
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"
export PLATFORM_TEST_REGISTRY="${REGISTRY_HOST}"

cat >"${TMPDIR_TEST}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${PLATFORM_TEST_REMOTE:-0}" == "1" ]]; then
  case "$1" in
    load) cat >/dev/null ;;
    push)
      [[ "${2:-}" == "${PLATFORM_TEST_REGISTRY}"/*:run-tag ]]
      printf 'digest: sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd size: 1\n'
      echo "remote docker push ${2}" >>"${PLATFORM_TEST_LOG}"
      ;;
    *) exit 1 ;;
  esac
  exit 0
fi
echo "docker $*" >>"${PLATFORM_TEST_LOG}"
if [[ "${1:-}" == "save" ]]; then
  printf 'image-stream\n'
fi
EOF

cat >"${TMPDIR_TEST}/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
remote_command=""
for argument in "$@"; do remote_command="${argument}"; done
PLATFORM_TEST_REMOTE=1 bash -c "${remote_command}"
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'Docker-Content-Digest: sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd\r\n'
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/ssh" "${TMPDIR_TEST}/curl"

export PATH="${TMPDIR_TEST}:${PATH}"
export PLATFORM_TEST_LOG="${TMPDIR_TEST}/calls.log"

bash "${REPO_ROOT}/test/e2e/prepare-platform-images.sh"

actual_pushes="$(grep '^remote docker push ' "${PLATFORM_TEST_LOG}" | sort)"
expected_pushes="$(printf '%s\n' \
  "remote docker push ${PLATFORM_TEST_REGISTRY}/karthos-backend:run-tag" \
  "remote docker push ${PLATFORM_TEST_REGISTRY}/karthos-frontend:run-tag" | sort)"
[[ "${actual_pushes}" == "${expected_pushes}" ]]

echo "platform image preparation uses only platform images: PASS"
