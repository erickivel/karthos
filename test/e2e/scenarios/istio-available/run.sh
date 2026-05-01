#!/usr/bin/env bash

set -euo pipefail

task test:e2e:scenario:istio

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

echo "Waiting 60s for mesh telemetry to flow through pipeline..."
sleep 60

lsof -ti :13133 | xargs kill 2>/dev/null || true
lsof -ti :18081 | xargs kill 2>/dev/null || true
kubectl port-forward -n oksg-system deployment/oksg-backend 13133:13133 &
PF_HEALTH_PID=$!
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080 &
PF_API_PID=$!
trap "kill $PF_HEALTH_PID $PF_API_PID 2>/dev/null || true" EXIT
sleep 3

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
assert not any("ops-toolbox" in (e.get("source", {}).get("labels", {}).get("name", "") or "") for e in mesh_edges), \
    "orphan pod should not emit mesh calls edges"
print(f"✓ API returned {len(mesh_edges)} mesh calls edge(s)")
PY
