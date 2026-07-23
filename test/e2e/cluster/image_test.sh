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
if [[ "$1" == "image" && "$2" == "inspect" ]]; then
  printf 'localhost:5001/demo@sha256:deadbeef\n'
fi
EOF

cat >"${TMPDIR_TEST}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "curl $*" >>"${IMAGE_TEST_LOG}"
if [[ "$*" == *"/manifests/demo-tag"* && "$*" == *"-fsSI"* ]]; then
  [[ "$*" == *"application/vnd.oci.image.manifest.v1+json"* ]] || exit 22
  if [[ -f "${IMAGE_TEST_TAGGED}" ]]; then exit 0; fi
  exit 22
fi
if [[ "$*" == *"/manifests/sha256:deadbeef"* ]]; then
  printf '{"schemaVersion":2}'
  exit 0
fi
if [[ " $* " == *" -X PUT "* ]]; then
  touch "${IMAGE_TEST_TAGGED}"
fi
EOF
chmod +x "${TMPDIR_TEST}/docker" "${TMPDIR_TEST}/curl"

export PATH="${TMPDIR_TEST}:${PATH}"
export IMAGE_TEST_LOG="${TMPDIR_TEST}/calls.log"
export IMAGE_TEST_TAGGED="${TMPDIR_TEST}/tagged"
export CLUSTER_PROFILE=remote-k3s

"${REPO_ROOT}/test/e2e/cluster/image.sh" push_or_load localhost:5001/demo:demo-tag

[[ -f "${IMAGE_TEST_TAGGED}" ]]
grep -q 'manifests/sha256:deadbeef' "${IMAGE_TEST_LOG}"
grep -q 'manifests/demo-tag' "${IMAGE_TEST_LOG}"
grep -q 'application/vnd.oci.image.manifest.v1+json' "${IMAGE_TEST_LOG}"

echo "image transport tag recovery: PASS"
