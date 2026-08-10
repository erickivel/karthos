#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t oksg-istio-run-test.XXXXXX)"
cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 && -f "${ISTIO_RUN_TEST_APPLY_MANIFEST:-}" ]]; then
    cat "${ISTIO_RUN_TEST_APPLY_MANIFEST}" >&2
  fi
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

export OKSG_IMAGE_TAG_OVERRIDE=run-tag
cd "${REPO_ROOT}"
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"
export ISTIO_RUN_TEST_REGISTRY="${REGISTRY_HOST}"

cat >"${TMPDIR_TEST}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  apply)
    [[ "${2:-}" == "-f" ]]
    cp "${3:?missing manifest}" "${ISTIO_RUN_TEST_APPLY_MANIFEST}"
    ;;
  logs)
    printf 'WARMUP_TRAFFIC_COMPLETE\nMEASURED_TRAFFIC_COMPLETE\n'
    ;;
  get)
    if [[ "${2:-}" == "pods" ]]; then
      printf 'mesh-protocol-server-0\n'
    fi
    ;;
esac
EOF

cat >"${TMPDIR_TEST}/task" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF

cat >"${TMPDIR_TEST}/python3" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF

cat >"${TMPDIR_TEST}/lsof" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF

cat >"${TMPDIR_TEST}/xargs" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
EOF

cat >"${TMPDIR_TEST}/sleep" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
EOF
chmod +x "${TMPDIR_TEST}/kubectl" "${TMPDIR_TEST}/task" "${TMPDIR_TEST}/python3" \
  "${TMPDIR_TEST}/lsof" "${TMPDIR_TEST}/xargs" "${TMPDIR_TEST}/sleep"

export PATH="${TMPDIR_TEST}:${PATH}"
export ISTIO_RUN_TEST_APPLY_MANIFEST="${TMPDIR_TEST}/applied.yaml"

bash "${REPO_ROOT}/test/e2e/scenarios/istio-available/run.sh"

grep -Fqx "          image: ${ISTIO_RUN_TEST_REGISTRY}/mesh-protocol-go:run-tag" "${ISTIO_RUN_TEST_APPLY_MANIFEST}"
if grep -Fq "${ISTIO_RUN_TEST_REGISTRY}/mesh-protocol-go:latest" "${ISTIO_RUN_TEST_APPLY_MANIFEST}"; then
  echo "stale mesh-protocol-go:latest image found in directly applied Job" >&2
  exit 1
fi

grep -Fq 'mesh["status"] == "available"' \
  "${REPO_ROOT}/test/e2e/scenarios/istio-available/run.sh"

echo "Istio protocol Job uses built fixture image tag: PASS"
