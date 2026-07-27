#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-image-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ "$status" -ne 0 && -f "${IMAGE_TEST_LOG:-}" ]]; then
    cat "${IMAGE_TEST_LOG}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "$status"
}
trap cleanup EXIT

cat >"${TMPDIR_TEST}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "docker $*" >>"${IMAGE_TEST_LOG}"
case "$1" in
  push)
    exit 0
    ;;
  save)
    if [[ "${IMAGE_TEST_MODE}" == "save_fail" ]]; then
      exit 42
    fi
    printf 'image-stream\n'
    ;;
  *)
    echo "unexpected local docker command: $*" >&2
    exit 99
    ;;
esac
EOF

cat >"${TMPDIR_TEST}/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "ssh $*" >>"${IMAGE_TEST_LOG}"

if [[ "$*" == *"docker load"* ]]; then
  [[ "$*" != *" -- localhost:5001/"* ]] || exit 45
  IFS= read -r image
  payload="$(cat)"
  [[ "${image}" == "localhost:5001/demo:demo-tag" ]] || exit 46
  [[ "${payload}" == "image-stream" ]] || exit 47
  [[ "$*" == *'docker push "$image"'* ]] || exit 48
  if [[ "$*" == *"docker load >&2"* ]]; then
    printf 'Loaded image: localhost:5001/demo:demo-tag\n' >&2
  else
    printf 'Loaded image: localhost:5001/demo:demo-tag\n'
  fi
  case "${IMAGE_TEST_MODE}" in
    load_fail) exit 43 ;;
    push_fail) exit 44 ;;
  esac
  echo "remote docker push ${image}" >>"${IMAGE_TEST_LOG}"
  printf '%s\n' "${IMAGE_TEST_PUSH_DIGEST}"
  exit 0
fi

[[ "$*" != *" -- demo demo-tag 5001"* ]] || exit 52
IFS= read -r repository
IFS= read -r tag
IFS= read -r registry_port
[[ "${repository}" == "demo" ]] || exit 49
[[ "${tag}" == "demo-tag" ]] || exit 50
[[ "${registry_port}" == "5001" ]] || exit 51
if [[ -n "${IMAGE_TEST_HEADER_DIGEST:-}" ]]; then
  printf 'Docker-Content-Digest: %s\r\n' "${IMAGE_TEST_HEADER_DIGEST}"
fi
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "curl $*" >>"${IMAGE_TEST_LOG}"
printf 'Docker-Content-Digest: sha256:kind\r\n'
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/ssh" "${TMPDIR_TEST}/curl"

export PATH="${TMPDIR_TEST}:${PATH}"
export IMAGE_TEST_LOG="${TMPDIR_TEST}/calls.log"
export IMAGE_TEST_PUSH_DIGEST="sha256:remote-push"
export IMAGE_TEST_HEADER_DIGEST="sha256:remote-push"

fixture_root=""
prepare_fixture() {
  local engine="$1"

  fixture_root="${TMPDIR_TEST}/${engine}"
  mkdir -p "${fixture_root}/test/e2e" "${fixture_root}/.env.d"
  cp -R "${REPO_ROOT}/test/e2e/cluster" "${fixture_root}/test/e2e/cluster"
  chmod +x "${fixture_root}/test/e2e/cluster/image.sh"
  printf 'CLUSTER_PROFILE=image-test\n' >"${fixture_root}/.env"
  {
    printf 'CLUSTER_ENGINE=%s\n' "${engine}"
    printf 'REGISTRY_HOST=localhost:5001\n'
    printf 'IMAGE_TAG=test\n'
    if [[ "${engine}" == "k3s" ]]; then
      printf 'CLUSTER_LOCATION=ssh\n'
      printf 'K3S_SSH_HOST=image-test-server\n'
    fi
  } >"${fixture_root}/.env.d/image-test.env"
  : >"${IMAGE_TEST_LOG}"
}

run_image() {
  "${fixture_root}/test/e2e/cluster/image.sh" push_or_load localhost:5001/demo:demo-tag
}

expect_failure() {
  local mode="$1"
  local expected_message="$2"

  prepare_fixture k3s
  IMAGE_TEST_MODE="${mode}"
  export IMAGE_TEST_MODE
  if run_image >"${TMPDIR_TEST}/${mode}.out" 2>&1; then
    echo "expected ${mode} transport to fail" >&2
    exit 1
  fi
  if [[ -n "${expected_message}" ]]; then
    grep -q "${expected_message}" "${TMPDIR_TEST}/${mode}.out"
  fi
}

prepare_fixture kind
export IMAGE_TEST_MODE=success
run_image
grep -q '^docker push localhost:5001/demo:demo-tag$' "${IMAGE_TEST_LOG}"
! grep -q '^ssh ' "${IMAGE_TEST_LOG}"

prepare_fixture k3s
export IMAGE_TEST_MODE=success
run_image
grep -q '^docker save localhost:5001/demo:demo-tag$' "${IMAGE_TEST_LOG}"
grep -q '^ssh ' "${IMAGE_TEST_LOG}"
grep -q '^remote docker push localhost:5001/demo:demo-tag$' "${IMAGE_TEST_LOG}"
! grep -q '^docker push ' "${IMAGE_TEST_LOG}"

export IMAGE_TEST_HEADER_DIGEST="sha256:other-manifest"
expect_failure success 'does not match registry manifest digest'

export IMAGE_TEST_HEADER_DIGEST=""
expect_failure success 'did not return Docker-Content-Digest'

export IMAGE_TEST_HEADER_DIGEST="sha256:remote-push"
expect_failure save_fail ''
expect_failure load_fail ''
expect_failure push_fail ''

echo "image transport: PASS"
