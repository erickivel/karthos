#!/usr/bin/env bash

set -euo pipefail

# Standalone commands below use kubectl directly, so load the same active
# cluster profile and KUBECONFIG as the Taskfile.
# shellcheck disable=SC1091
. "$(pwd)/test/e2e/cluster/profile.sh"

task test:e2e:scenario:istio:prepare

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
mesh = next((s for s in profile["sources"] if s["type"] == "istio"), None)
assert mesh is not None, "No istio source found in profile"
assert mesh["present"] is True, "Istio source should be present"
assert mesh["mesh"]["flavor"] == "istio"
assert mesh["mesh"]["mode"] == "sidecar"
assert "oksg-demo" in mesh["mesh"].get("injected_namespaces", [])
print("✓ Discovery profile includes Istio mesh source")
PY

python3 - <<'PY'
from pathlib import Path

values = Path("bin/e2e/otel-values.yaml").read_text()
assert "job_name: mesh-istio" in values, "generated values should include mesh-istio scrape job"
assert "istio-proxy" in values, "mesh-istio scrape job should keep istio-proxy containers"
assert "oksg_source" in values, "mesh scrape should stamp normalized source"
print("✓ Generated collector values include Istio scrape wiring")
PY

kubectl delete job mesh-protocol-client -n oksg-demo --ignore-not-found
RENDERED_PROTOCOL_CLIENT_JOB="$(mktemp -t oksg-mesh-protocol-client.XXXXXX)"
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
  if kubectl logs -n oksg-demo job/mesh-protocol-client -c mesh-protocol-client 2>/dev/null | grep -q WARMUP_TRAFFIC_COMPLETE; then
    warmup_ready=true
    break
  fi
  sleep 2
done
if [[ "${warmup_ready}" != "true" ]]; then
  kubectl logs -n oksg-demo job/mesh-protocol-client -c mesh-protocol-client || true
  echo "protocol warm-up did not complete" >&2
  exit 1
fi

lsof -ti :13133 | xargs kill 2>/dev/null || true
lsof -ti :18081 | xargs kill 2>/dev/null || true
lsof -ti :18428 | xargs kill 2>/dev/null || true
lsof -ti :19020 | xargs kill 2>/dev/null || true
MESH_SERVER_POD="$(kubectl get pods -n oksg-demo -l app=mesh-protocol-server -o jsonpath='{.items[0].metadata.name}')"
kubectl port-forward -n oksg-system deployment/oksg-backend 13133:13133 &
PF_HEALTH_PID=$!
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080 &
PF_API_PID=$!
kubectl port-forward -n oksg-system svc/oksg-otel-victoria-metrics-single-server 18428:8428 &
PF_VM_PID=$!
kubectl port-forward -n oksg-demo "pod/${MESH_SERVER_POD}" 19020:15020 &
PF_MESH_PID=$!
trap "kill $PF_HEALTH_PID $PF_API_PID $PF_VM_PID $PF_MESH_PID 2>/dev/null || true" EXIT
sleep 3

python3 test/e2e/scenarios/istio-available/verify_protocol_metrics.py capture-baseline

echo "Waiting for deterministic measured gRPC/TCP traffic..."
measured_ready=false
for _ in {1..90}; do
  if kubectl logs -n oksg-demo job/mesh-protocol-client -c mesh-protocol-client 2>/dev/null | grep -q MEASURED_TRAFFIC_COMPLETE; then
    measured_ready=true
    break
  fi
  sleep 2
done
if [[ "${measured_ready}" != "true" ]]; then
  kubectl logs -n oksg-demo job/mesh-protocol-client -c mesh-protocol-client || true
  echo "measured protocol traffic did not complete" >&2
  exit 1
fi

echo "Waiting for the measured counters to cross the collector scrape boundary..."
python3 test/e2e/scenarios/istio-available/verify_protocol_metrics.py verify

python3 - <<'PY'
import json
import urllib.request

graph = json.loads(urllib.request.urlopen("http://localhost:18081/api/v1/graph/edges?relation=calls&page_size=1000").read())
edges = graph.get("data", {}).get("edges", [])
assert edges, "expected calls edges from mesh telemetry"
mesh_edges = [e for e in edges if "mesh.istio" in e.get("provenance", [])]
assert mesh_edges, "expected at least one mesh.istio-provenance edge"
assert any(e.get("metadata", {}).get("mesh.security.policy") == "mutual_tls" for e in mesh_edges), \
    "expected at least one mutual_tls mesh edge"
snapshot = json.loads(urllib.request.urlopen("http://localhost:18081/api/v1/graph").read())
nodes = snapshot.get("data", {}).get("graph", {}).get("nodes", [])
node_names = {node.get("id"): node.get("name", "") for node in nodes}
assert not any(
    "ops-toolbox" in node_names.get(endpoint, "")
    for edge in mesh_edges
    for endpoint in (edge.get("source"), edge.get("target"))
), \
    "orphan pod should not emit mesh calls edges"
print(f"✓ API returned {len(mesh_edges)} mesh calls edge(s)")
PY
