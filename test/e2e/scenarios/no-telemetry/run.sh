#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
E2E_OUTPUT_DIR="${OKSG_E2E_OUTPUT_DIR:-${REPO_ROOT}/bin/e2e}"

cd "${REPO_ROOT}"
task test:e2e:scenario:fallback:prepare

python3 - "${E2E_OUTPUT_DIR}" <<'PY'
import json
import sys
from pathlib import Path

output_dir = Path(sys.argv[1])
profile = json.loads((output_dir / "profile.json").read_text())
plan = json.loads((output_dir / "source-plan.json").read_text())
values = (output_dir / "otel-values.yaml").read_text()

capabilities = {capability["source_id"]: capability for capability in profile["capabilities"]}
for source_id in ("kubernetes.structure", "kubelet", "host-os", "kube-cluster"):
    assert capabilities[source_id]["status"] == "available", source_id
for source_id in ("prometheus.app", "mesh.istio", "traces.servicegraph"):
    assert capabilities[source_id]["status"] == "unavailable", source_id

assert capabilities["prometheus.app"].get("evidence", {}).get("prometheus_targets", []) == []
assert set(plan["selected_sources"]) == {"host-os", "kube-cluster", "kubelet"}
for receiver in ("kubeletstats", "hostmetrics", "k8s_cluster"):
    assert receiver in values, receiver
PY
