#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
TMPDIR_TEST="$(mktemp -d -t karthos-fallback-run-test.XXXXXX)"
cleanup() {
  local status=$?
  rm -rf "${TMPDIR_TEST}"
  exit "${status}"
}
trap cleanup EXIT

mkdir -p "${TMPDIR_TEST}/bin" "${TMPDIR_TEST}/work"
cat >"${TMPDIR_TEST}/bin/task" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"${FALLBACK_TEST_TASK_LOG}"
EOF
chmod +x "${TMPDIR_TEST}/bin/task"

cat >"${TMPDIR_TEST}/profile.json" <<'EOF'
{
  "capabilities": [
    {"source_id":"kubernetes.structure","status":"available"},
    {"source_id":"kubelet","status":"available"},
    {"source_id":"host-os","status":"available"},
    {"source_id":"kube-cluster","status":"available"},
    {"source_id":"prometheus.app","status":"unavailable","evidence":{"prometheus_targets":[]}},
    {"source_id":"mesh.istio","status":"unavailable"},
    {"source_id":"traces.servicegraph","status":"unavailable"}
  ]
}
EOF
cat >"${TMPDIR_TEST}/otel-values.yaml.source-plan.json" <<'EOF'
{"selected_sources":["host-os","kube-cluster","kubelet"]}
EOF
cat >"${TMPDIR_TEST}/otel-values.yaml" <<'EOF'
receivers:
  kubelet_stats: {}
  host_metrics: {}
  k8s_cluster: {}
EOF

export PATH="${TMPDIR_TEST}/bin:${PATH}"
export FALLBACK_TEST_TASK_LOG="${TMPDIR_TEST}/task.log"
export KARTHOS_E2E_OUTPUT_DIR="${TMPDIR_TEST}"

cd "${TMPDIR_TEST}/work"
bash "${REPO_ROOT}/test/e2e/scenarios/no-telemetry/run.sh"

[[ ! -s "${FALLBACK_TEST_TASK_LOG}" ]]

echo "fallback runner validates the strict direct-source contract: PASS"
