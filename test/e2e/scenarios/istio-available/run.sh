#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
cd "${REPO_ROOT}"

# Standalone commands below use kubectl directly, so load the same active
# cluster profile and KUBECONFIG as the Taskfile.
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

NAMESPACE="${KIND_NAMESPACE:-karthos-system}"
BACKEND_DEPLOYMENT="${HELM_RELEASE_NAME:-karthos}-backend"
VM_SERVICE="${HELM_RELEASE_NAME:-karthos}-victoria-metrics-single-server"
GATEWAY_LOCAL_PORT="${GATEWAY_LOCAL_PORT:-18080}"

require_open_port() {
  local port="$1"
  if lsof -ti ":${port}" >/dev/null 2>&1; then
    echo "local port ${port} is busy; stop the conflicting process before running this live scenario" >&2
    exit 1
  fi
}

cleanup_port_forwards() {
  local pid
  for pid in "$@"; do
    kill "${pid}" 2>/dev/null || true
  done
  for _ in $(seq 1 20); do
    for pid in "$@"; do
      if kill -0 "${pid}" 2>/dev/null; then
        sleep 0.1
        continue 2
      fi
    done
    break
  done
  for pid in "$@"; do
    if kill -0 "${pid}" 2>/dev/null; then
      kill -KILL "${pid}" 2>/dev/null || true
    fi
    wait "${pid}" 2>/dev/null || true
  done
}

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
mesh = next((c for c in profile["capabilities"] if c["source_id"] == "mesh.istio"), None)
assert mesh is not None, "No istio capability found in profile"
assert mesh["status"] == "available", "Istio capability should be available"
evidence = mesh["evidence"]["mesh"]
assert evidence["flavor"] == "istio"
assert evidence["mode"] == "sidecar"
assert "karthos-demo" in evidence.get("injected_namespaces", [])
print("✓ Capability profile includes Istio mesh")
PY

python3 - <<'PY'
from pathlib import Path

values = Path("bin/e2e/otel-values.yaml").read_text()
assert "job_name: mesh-istio" in values, "generated values should include mesh-istio scrape job"
assert "istio-proxy" in values, "mesh-istio scrape job should keep istio-proxy containers"
assert "karthos_source" in values, "mesh scrape should stamp normalized source"
print("✓ Generated collector values include Istio scrape wiring")
PY

kubectl delete job mesh-protocol-client -n karthos-demo --ignore-not-found
RENDERED_PROTOCOL_CLIENT_JOB="$(mktemp -t karthos-mesh-protocol-client.XXXXXX)"
REGISTRY_HOST_PATTERN="${REGISTRY_HOST//./\\.}"
sed "s|${REGISTRY_HOST_PATTERN}/mesh-protocol-go:latest|${REGISTRY_HOST}/mesh-protocol-go:${IMAGE_TAG}|g" \
  test/e2e/apps/with-istio/mesh-protocol-client-job.yaml >"${RENDERED_PROTOCOL_CLIENT_JOB}"
if ! kubectl apply -f "${RENDERED_PROTOCOL_CLIENT_JOB}"; then
  rm -f "${RENDERED_PROTOCOL_CLIENT_JOB}"
  exit 1
fi
rm -f "${RENDERED_PROTOCOL_CLIENT_JOB}"

echo "Waiting for deterministic protocol warm-up..."
warmup_ready=false
for _ in {1..60}; do
  if kubectl logs -n karthos-demo job/mesh-protocol-client -c mesh-protocol-client 2>/dev/null | grep -q WARMUP_TRAFFIC_COMPLETE; then
    warmup_ready=true
    break
  fi
  sleep 2
done
if [[ "${warmup_ready}" != "true" ]]; then
  kubectl logs -n karthos-demo job/mesh-protocol-client -c mesh-protocol-client || true
  echo "protocol warm-up did not complete" >&2
  exit 1
fi

require_open_port 13133
require_open_port 18081
require_open_port 18428
require_open_port 19020
MESH_SERVER_POD="$(kubectl get pods -n karthos-demo -l app=mesh-protocol-server -o jsonpath='{.items[0].metadata.name}')"
kubectl port-forward -n "${NAMESPACE}" deployment/${BACKEND_DEPLOYMENT} 13133:13133 &
PF_HEALTH_PID=$!
kubectl port-forward -n "${NAMESPACE}" deployment/${BACKEND_DEPLOYMENT} 18081:8080 &
PF_API_PID=$!
kubectl port-forward -n "${NAMESPACE}" svc/${VM_SERVICE} 18428:8428 &
PF_VM_PID=$!
kubectl port-forward -n karthos-demo "pod/${MESH_SERVER_POD}" 19020:15020 &
PF_MESH_PID=$!
trap 'cleanup_port_forwards "${PF_HEALTH_PID}" "${PF_API_PID}" "${PF_VM_PID}" "${PF_MESH_PID}"' EXIT
trap 'exit 130' INT TERM
sleep 3

python3 test/e2e/scenarios/istio-available/verify_protocol_metrics.py capture-baseline

echo "Waiting for deterministic measured gRPC/TCP traffic..."
measured_ready=false
for _ in {1..90}; do
  if kubectl logs -n karthos-demo job/mesh-protocol-client -c mesh-protocol-client 2>/dev/null | grep -q MEASURED_TRAFFIC_COMPLETE; then
    measured_ready=true
    break
  fi
  sleep 2
done
if [[ "${measured_ready}" != "true" ]]; then
  kubectl logs -n karthos-demo job/mesh-protocol-client -c mesh-protocol-client || true
  echo "measured protocol traffic did not complete" >&2
  exit 1
fi

echo "Waiting for the measured counters to cross the collector scrape boundary..."
python3 test/e2e/scenarios/istio-available/verify_protocol_metrics.py verify

python3 - <<'PY'
import json
import time
import urllib.request

deadline = time.time() + 330
edges = []
mesh_edges = []
nodes = []
while time.time() < deadline:
    graph = json.loads(urllib.request.urlopen(
        "http://localhost:18081/api/v1/graph/edges?relation=calls&page_size=1000",
        timeout=5,
    ).read())
    edges = graph.get("data", {}).get("edges", [])
    mesh_edges = [e for e in edges if "mesh.istio" in e.get("provenance", [])]
    if any(e.get("metadata", {}).get("mesh.security.policy") == "mutual_tls" for e in mesh_edges):
        snapshot = json.loads(urllib.request.urlopen("http://localhost:18081/api/v1/graph", timeout=5).read())
        nodes = snapshot.get("data", {}).get("graph", {}).get("nodes", [])
        break
    time.sleep(5)

assert edges, "expected calls edges from mesh telemetry after one materialization interval"
assert mesh_edges, "expected at least one mesh.istio-provenance edge after one materialization interval"
assert any(e.get("metadata", {}).get("mesh.security.policy") == "mutual_tls" for e in mesh_edges), \
    "expected at least one mutual_tls mesh edge after one materialization interval"
node_names = {node.get("id"): node.get("name", "") for node in nodes}
assert not any(
    "ops-toolbox" in node_names.get(endpoint, "")
    for edge in mesh_edges
    for endpoint in (edge.get("source"), edge.get("target"))
), \
    "orphan pod should not emit mesh calls edges"
print(f"✓ API returned {len(mesh_edges)} mesh calls edge(s)")
PY
