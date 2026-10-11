#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-verify-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin"

cat >"${TMPDIR_TEST}/bin/helm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF

cat >"${TMPDIR_TEST}/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"get deployment/karthos-backend"*)
    printf 'localhost:5001/karthos-backend:test-tag'
    ;;
  *"get deployment/karthos-frontend"*)
    printf 'localhost:5001/karthos-frontend:test-tag'
    ;;
  *"get pods"*"component=backend"*)
    printf 'localhost:5001/karthos-backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    ;;
  *"get pods"*"component=frontend"*)
    printf 'localhost:5001/karthos-frontend@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
    ;;
esac
EOF

cat >"${TMPDIR_TEST}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
expected_accept='Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'
if [[ "$#" -ne 4 || "$1" != "-fsSI" || "$2" != "-H" || "$3" != "${expected_accept}" ]]; then
  printf 'curl received split arguments:' >&2
  printf ' <%s>' "$@" >&2
  printf '\n' >&2
  exit 64
fi
case "$4" in
  */karthos-backend/*)
    printf 'Docker-Content-Digest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\n'
    ;;
  */karthos-frontend/*)
    printf 'Docker-Content-Digest: sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\r\n'
    ;;
  *)
    printf 'unexpected registry URL: %s\n' "$4" >&2
    exit 65
    ;;
esac
EOF

cat >"${TMPDIR_TEST}/bin/ssh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "-o" && "$2" == "BatchMode=yes" && "$3" == "kivel-server" ]]
shift 3
bash -c "$*"
EOF

chmod +x "${TMPDIR_TEST}/bin/"*

PATH="${TMPDIR_TEST}/bin:${PATH}" \
  CLUSTER_ENGINE=k3s \
  K3S_SSH_HOST=kivel-server \
  IMAGE_TAG=test-tag \
  REGISTRY_HOST=localhost:5001 \
  KIND_NAMESPACE=karthos-system \
  HELM_RELEASE_NAME=karthos \
  bash "${REPO_ROOT}/test/e2e/platform/verify.sh"

echo "remote registry verification preserves the complete Accept header: PASS"
