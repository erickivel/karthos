#!/usr/bin/env bash

set -euo pipefail

task test:e2e:scenario:prometheus

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
expected = json.loads(Path("test/e2e/scenarios/prometheus-available/expected-profile.json").read_text())

assert profile["recommended_receivers"] == expected["recommended_receivers"], \
    f"recommended_receivers mismatch: {profile['recommended_receivers']} != {expected['recommended_receivers']}"

prom_source = next((s for s in profile["sources"] if s["type"] == "prometheus"), None)
assert prom_source is not None, "No prometheus source found in profile"
assert prom_source["present"] is expected["source_expectations"]["present"]
assert len(prom_source["prometheus_targets"]) >= 1, "Expected at least one prometheus target"

print("✓ Discovery profile valid")
PY

# Wait for telemetry to flow through the pipeline.
echo "Waiting 60s for telemetry to flow through pipeline..."
sleep 60

# Port-forward to backend health endpoint.
lsof -ti :13133 | xargs kill 2>/dev/null || true
kubectl port-forward -n oksg-system deployment/oksg-backend 13133:13133 &
PF_PID=$!
trap "kill $PF_PID 2>/dev/null || true" EXIT
sleep 3

# Verify backend received metric samples and inspect the graph.
python3 - <<'PY'
import json
import urllib.request
import sys

metrics_resp = urllib.request.urlopen("http://localhost:13133/debug/metrics")
metrics = json.loads(metrics_resp.read())
assert len(metrics) > 0, f"Expected metric samples in backend, got empty list"
names = sorted({m["metric_name"] for m in metrics})
print(f"✓ Backend has {len(metrics)} metric samples")
print(f"  First 5 metric names: {names[:5]}")

graph_resp = urllib.request.urlopen("http://localhost:13133/debug/graph")
graph = json.loads(graph_resp.read())
if graph:
    edges = graph.get("graph", {}).get("edges", [])
    nodes = graph.get("graph", {}).get("nodes", [])
    print(f"✓ Backend graph: {len(nodes)} nodes, {len(edges)} edges")
    if edges:
        relations = sorted({e.get("relation") for e in edges})
        print(f"  Edge relations: {relations}")
else:
    print("⚠ No graph snapshot yet (traces may not have flowed)")
PY

echo "✓ E2E prometheus scenario passed"
