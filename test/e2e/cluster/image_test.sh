#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-image-test.XXXXXX)"
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
if [[ "${IMAGE_TEST_REMOTE:-0}" == "1" ]]; then
  echo "remote docker $*" >>"${IMAGE_TEST_LOG}"
  case "$1" in
    load)
      [[ "$(cat)" == "image-stream" ]] || exit 47
      if [[ "${IMAGE_TEST_MODE}" == "load_fail" ]]; then
        exit 43
      fi
      printf 'Loaded image: localhost:5001/demo:demo-tag\n'
      ;;
    push)
      [[ "${2:-}" == "localhost:5001/demo:demo-tag" ]] || exit 48
      if [[ "${IMAGE_TEST_MODE}" == "push_fail" ]]; then
        exit 44
      fi
      echo "remote docker push ${2}" >>"${IMAGE_TEST_LOG}"
      printf 'digest: %s size: 1\n' "${IMAGE_TEST_PUSH_DIGEST}"
      ;;
    *)
      echo "unexpected remote docker command: $*" >&2
      exit 99
      ;;
  esac
  exit 0
fi
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

remote_command=""
for argument in "$@"; do
  remote_command="${argument}"
done
case "${remote_command}" in
  "bash -c "*) ;;
  *) exit 45 ;;
esac
[[ "${remote_command}" == *"set -euo pipefail"* ]] || exit 46
[[ "${remote_command}" != *" -- localhost:5001/"* ]] || exit 52
[[ "${remote_command}" != *" -- demo demo-tag 5001"* ]] || exit 53
IMAGE_TEST_REMOTE=1 bash -c "${remote_command}"
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "curl $*" >>"${IMAGE_TEST_LOG}"
[[ "${IMAGE_TEST_REMOTE:-0}" == "1" ]] || exit 99
if [[ -n "${IMAGE_TEST_HEADER_DIGEST:-}" ]]; then
  printf 'Docker-Content-Digest: %s\r\n' "${IMAGE_TEST_HEADER_DIGEST}"
fi
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/ssh" "${TMPDIR_TEST}/curl"

export PATH="${TMPDIR_TEST}:${PATH}"
export IMAGE_TEST_LOG="${TMPDIR_TEST}/calls.log"
export IMAGE_TEST_PUSH_DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export IMAGE_TEST_HEADER_DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
if grep -q '^ssh ' "${IMAGE_TEST_LOG}"; then
  echo "kind transport unexpectedly used ssh" >&2
  exit 1
fi

prepare_fixture k3s
export IMAGE_TEST_MODE=success
run_image
grep -q '^docker save localhost:5001/demo:demo-tag$' "${IMAGE_TEST_LOG}"
grep -q '^ssh ' "${IMAGE_TEST_LOG}"
grep -q '^remote docker push localhost:5001/demo:demo-tag$' "${IMAGE_TEST_LOG}"
if grep -q '^docker push ' "${IMAGE_TEST_LOG}"; then
  echo "k3s transport unexpectedly used local docker push" >&2
  exit 1
fi

export IMAGE_TEST_HEADER_DIGEST="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
expect_failure success 'does not match registry manifest digest'

export IMAGE_TEST_HEADER_DIGEST=""
expect_failure success 'invalid remote registry manifest digest'

export IMAGE_TEST_HEADER_DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
expect_failure save_fail ''
expect_failure load_fail ''
expect_failure push_fail ''

export IMAGE_TEST_PUSH_DIGEST="sha256:short"
export IMAGE_TEST_HEADER_DIGEST="sha256:short"
expect_failure success 'invalid remote push digest'

export IMAGE_TEST_PUSH_DIGEST="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
export IMAGE_TEST_HEADER_DIGEST="sha256:short"
expect_failure success 'invalid remote registry manifest digest'

echo "image transport: PASS"
