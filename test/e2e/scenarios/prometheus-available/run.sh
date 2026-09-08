#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
cd "${REPO_ROOT}"

# Standalone commands below use kubectl directly, so load the same active
# cluster profile and KUBECONFIG as the Taskfile.
# shellcheck disable=SC1091
. "${REPO_ROOT}/test/e2e/cluster/profile.sh"

NAMESPACE="${KIND_NAMESPACE:-oksg-system}"
BACKEND_DEPLOYMENT="${HELM_RELEASE_NAME:-oksg-otel}-backend"
VM_SERVICE="${HELM_RELEASE_NAME:-oksg-otel}-victoria-metrics-single-server"
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

task test:e2e:scenario:prometheus:prepare

python3 - <<'PY'
import json
from pathlib import Path

profile = json.loads(Path("bin/e2e/profile.json").read_text())
expected = json.loads(Path("test/e2e/scenarios/prometheus-available/expected-profile.json").read_text())

prom_source = next((c for c in profile["capabilities"] if c["source_id"] == expected["source_id"]), None)
assert prom_source is not None, "No prometheus capability found in profile"
assert prom_source["status"] == expected["status"]
assert len(prom_source.get("evidence", {}).get("prometheus_targets", [])) >= 1, "Expected at least one prometheus target"

print("✓ Capability profile valid")
PY

# Verify the generated collector values wire up the selected servicegraph connector.
python3 - <<'PY'
from pathlib import Path

values = Path("bin/e2e/otel-values.yaml").read_text()
assert "service_graph:" in values, "generated Helm values should include the service_graph connector"
assert "otlp_grpc:" in values, "generated Helm values should use the OTLP gRPC exporter ID"
assert 'set(datapoint.attributes["oksg_source"]' in values, "source tagging must use explicit datapoint context"
assert 'set(attributes["oksg_source"]' not in values, "ambiguous OTTL attribute paths must not be generated"
assert "metrics_flush_interval: 15s" in values, "servicegraph should flush on the configured 15s ticker"
print("✓ Generated collector values include service_graph wiring")
PY

# Create every success/error branch, then let the cumulative series span the
# minimum query window before capturing the controlled-traffic baseline.
task test:e2e:traffic GATEWAY_LOCAL_PORT="${GATEWAY_LOCAL_PORT}" TRAFFIC_REQUESTS=1 TRAFFIC_ERROR_REQUESTS=1 TRAFFIC_INTERVAL=0.2 TRAFFIC_INTERLEAVE_ERRORS=true
sleep 65

# Port-forward before traffic so the v0.156 servicegraph cumulative-counter
# baselines can be captured. The exact raw truth is the post-traffic increase.
require_open_port 13133
require_open_port 18081
require_open_port 18428
kubectl port-forward -n "${NAMESPACE}" deployment/${BACKEND_DEPLOYMENT} 13133:13133 &
PF_HEALTH_PID=$!
kubectl port-forward -n "${NAMESPACE}" deployment/${BACKEND_DEPLOYMENT} 18081:8080 &
PF_API_PID=$!
kubectl port-forward -n "${NAMESPACE}" svc/${VM_SERVICE} 18428:8428 &
PF_VM_PID=$!
trap 'cleanup_port_forwards "${PF_HEALTH_PID}" "${PF_API_PID}" "${PF_VM_PID}"' EXIT
trap 'exit 130' INT TERM
sleep 3

python3 - <<'PY'
import json
import time
import urllib.parse
import urllib.request
from pathlib import Path

expected_edges = {
    ("gateway-go", "service-a-python"),
    ("gateway-go", "service-b-node"),
    ("service-a-python", "service-b-node"),
}

def query(metric):
    selectors = " or ".join(
        f'{metric}{{source="traces.servicegraph",temporality!="delta",oksg_unit="count",edge_client="{client}",edge_server="{server}"}}'
        for client, server in sorted(expected_edges)
    )
    promql = (
        'sum by (edge_client,edge_server) '
        f'({selectors})'
    )
    url = "http://localhost:18428/api/v1/query?" + urllib.parse.urlencode({"query": promql})
    result = json.loads(urllib.request.urlopen(url, timeout=5).read())["data"]["result"]
    return [
        {
            "edge_client": item["metric"].get("edge_client"),
            "edge_server": item["metric"].get("edge_server"),
            "value": float(item["value"][1]),
        }
        for item in result
    ]

deadline = time.time() + 120
requests = []
errors = []
while time.time() < deadline:
    requests = query("servicegraph_request_count")
    errors = query("servicegraph_request_error_count")
    request_edges = {(item["edge_client"], item["edge_server"]) for item in requests}
    error_edges = {(item["edge_client"], item["edge_server"]) for item in errors}
    if request_edges == expected_edges and error_edges == expected_edges:
        break
    time.sleep(3)

request_edges = {(item["edge_client"], item["edge_server"]) for item in requests}
error_edges = {(item["edge_client"], item["edge_server"]) for item in errors}
assert request_edges == expected_edges, f"incomplete servicegraph request baseline: {requests}"
assert error_edges == expected_edges, f"incomplete servicegraph error baseline: {errors}"

Path("bin/e2e/servicegraph-baseline.json").write_text(json.dumps({
    "requests": requests,
    "errors": errors,
}))
print("✓ Captured servicegraph cumulative-counter baselines")
PY

# Generate one bounded traffic window so every rate and ratio has a known truth set.
task test:e2e:traffic GATEWAY_LOCAL_PORT="${GATEWAY_LOCAL_PORT}" TRAFFIC_REQUESTS=30 TRAFFIC_ERROR_REQUESTS=10 TRAFFIC_INTERVAL=0.5 TRAFFIC_INTERLEAVE_ERRORS=true

# Inspect the graph through the public API. Calls are current observations, so
# they become visible only when the backend publishes the next materialized
# graph snapshot (the supported production cadence is five minutes).
python3 - <<'PY'
import json
import time
import urllib.request

controlled_participants = {
    ("oksg-demo", "gateway-go"),
    ("oksg-demo", "service-a-python"),
    ("oksg-demo", "service-b-node"),
}
expected_calls = [
    (("oksg-demo", "gateway-go"), ("oksg-demo", "service-a-python")),
    (("oksg-demo", "gateway-go"), ("oksg-demo", "service-b-node")),
    (("oksg-demo", "service-a-python"), ("oksg-demo", "service-b-node")),
]
deadline = time.time() + 330
graph = {}
nodes = []
edges = []
all_calls = []
calls = []
while time.time() < deadline:
    graph_resp = urllib.request.urlopen("http://localhost:18081/api/v1/graph", timeout=5)
    graph = json.loads(graph_resp.read())["data"]
    edges = graph.get("graph", {}).get("edges", [])
    nodes = graph.get("graph", {}).get("nodes", [])
    identities = {
        node["id"]: (
            node.get("labels", {}).get("namespace", ""),
            node.get("labels", {}).get("name", ""),
        )
        for node in nodes
    }
    all_calls = [
        (
            identities.get(edge.get("source"), ("", "")),
            identities.get(edge.get("target"), ("", "")),
        )
        for edge in edges
        if edge.get("relation") == "calls"
    ]
    calls = [
        edge
        for edge in all_calls
        if edge[0] in controlled_participants and edge[1] in controlled_participants
    ]
    if sorted(calls) == sorted(expected_calls):
        break
    time.sleep(5)

assert nodes and edges, f"expected non-empty graph snapshot, got {len(nodes)} nodes and {len(edges)} edges"
assert sorted(calls) == sorted(expected_calls), \
    f"calls graph gate mismatch after waiting for one materialization interval: {calls} != {expected_calls}"
print(f"✓ Backend graph contains the exact three controlled calls edges ({len(nodes)} nodes total)")

fixture_nodes = {
    (node.get("kind"), node.get("labels", {}).get("namespace"), node.get("labels", {}).get("name")): node["id"]
    for node in nodes
}
namespace = fixture_nodes[("namespace", "oksg-demo", "oksg-demo")]
workload = fixture_nodes[("workload", "oksg-demo", "config-bundle")]
config_map = fixture_nodes[("configmap", "oksg-demo", "bundle-config")]
secret = fixture_nodes[("secret", "oksg-demo", "bundle-secret")]
pvc = fixture_nodes[("pvc", "oksg-demo", "bundle-cache")]
relations = {(edge.get("source"), edge.get("target"), edge.get("relation")) for edge in edges}
assert (namespace, workload, "contains") in relations
assert (workload, config_map, "uses") in relations
assert (workload, secret, "uses") in relations
assert (workload, pvc, "mounts") in relations
print("✓ Structural fixture bundle preserves Namespace containment and workload references")
unrelated_calls = [edge for edge in all_calls if edge not in calls]
if unrelated_calls:
    print(f"  diagnostic: ignored {len(unrelated_calls)} unrelated stored calls edge(s): {sorted(unrelated_calls)}")
for relation in sorted({edge.get("relation") for edge in edges} - {"calls"}):
    count = sum(1 for edge in edges if edge.get("relation") == relation)
    print(f"  diagnostic: {count} '{relation}' edge(s)")
PY

# Validate raw delta totals in VictoriaMetrics, then verify the public API's
# temporality-aware rates and duration means.
python3 - <<'PY'
import json
import math
import time
import urllib.parse
import urllib.request
from pathlib import Path

expected_edges = {
    ("gateway-go", "service-a-python"),
    ("gateway-go", "service-b-node"),
    ("service-a-python", "service-b-node"),
}
def vm_query(promql):
    url = "http://localhost:18428/api/v1/query?" + urllib.parse.urlencode({"query": promql})
    payload = json.loads(urllib.request.urlopen(url, timeout=5).read())
    assert payload.get("status") == "success", payload
    return payload["data"]["result"]

def edge_values(result):
    return {
        (item["metric"].get("edge_client"), item["metric"].get("edge_server")): float(item["value"][1])
        for item in result
    }

def exact_edge_query(metric):
    selectors = " or ".join(
        f'{metric}{{source="traces.servicegraph",temporality!="delta",oksg_unit="count",edge_client="{client}",edge_server="{server}"}}'
        for client, server in sorted(expected_edges)
    )
    return f"sum by (edge_client,edge_server) ({selectors})"

def baseline_values(name):
    payload = json.loads(Path("bin/e2e/servicegraph-baseline.json").read_text())
    return {
        (item["edge_client"], item["edge_server"]): float(item["value"])
        for item in payload[name]
    }

baseline_requests = baseline_values("requests")
baseline_errors = baseline_values("errors")

deadline = time.time() + 120
raw_requests = {}
raw_errors = {}
while time.time() < deadline:
    request_query = exact_edge_query("servicegraph_request_count")
    error_query = exact_edge_query("servicegraph_request_error_count")
    current_requests = edge_values(vm_query(request_query))
    current_errors = edge_values(vm_query(error_query))
    raw_requests = {edge: value - baseline_requests.get(edge, 0) for edge, value in current_requests.items()}
    raw_errors = {edge: value - baseline_errors.get(edge, 0) for edge, value in current_errors.items()}
    if all(raw_requests.get(edge) == 40 for edge in expected_edges) and all(raw_errors.get(edge) == 10 for edge in expected_edges):
        break
    time.sleep(5)

assert set(raw_requests) == expected_edges, f"unexpected request edges: {raw_requests}"
assert set(raw_errors) == expected_edges, f"unexpected error edges: {raw_errors}"
for edge in sorted(expected_edges):
    assert raw_requests[edge] == 40, f"{edge} request total: {raw_requests[edge]} != 40"
    assert raw_errors[edge] == 10, f"{edge} error total: {raw_errors[edge]} != 10"
print("✓ VM raw cumulative-counter increases match 40 requests and 10 errors on all internal edges")

def api_payload(metric, aggregation=None, window="1m"):
    params = {
        "metric_name": metric,
        "page_size": "1000",
    }
    if aggregation:
        params["aggregation"] = aggregation
    if window:
        params["window"] = window
    url = "http://localhost:18081/api/v1/metrics?" + urllib.parse.urlencode(params)
    payload = json.loads(urllib.request.urlopen(url, timeout=5).read())
    return payload.get("data", payload)

def api_samples(metric, aggregation=None, window="1m"):
    return api_payload(metric, aggregation, window).get("samples", [])

def assert_evaluated_query(payload, aggregation, window):
    query = payload["query"]
    assert query["aggregation"] == aggregation, query
    assert query["window"] == window, query
    assert query["step"] == "15s", query
    assert query["timestamp_semantics"] == "evaluation", query
    assert query["from"] <= query["to"], query

def latest_by_edge(samples):
    latest = {}
    for sample in samples:
        labels = sample.get("labels", {})
        if labels.get("source") != "traces.servicegraph":
            continue
        edge = (labels.get("edge_client"), labels.get("edge_server"))
        if edge not in expected_edges:
            continue
        if edge not in latest or sample["timestamp"] > latest[edge]["timestamp"]:
            latest[edge] = sample
    return latest

def latest_aggregate_by_edge(samples):
    grouped = {}
    for sample in samples:
        labels = sample.get("labels", {})
        if labels.get("source") != "traces.servicegraph":
            continue
        edge = (labels.get("edge_client"), labels.get("edge_server"))
        if edge not in expected_edges:
            continue
        timestamp = sample["timestamp"]
        current = grouped.get(edge)
        if current is None or timestamp > current[0]:
            grouped[edge] = (timestamp, float(sample["value"]))
        elif timestamp == current[0]:
            grouped[edge] = (timestamp, current[1] + float(sample["value"]))
    return {edge: value for edge, (_, value) in grouped.items()}

api_deadline = time.time() + 120
request_increase_payload = {}
error_increase_payload = {}
default_requests_payload = {}
error_rate_payload = {}
active_rate_payload = {}
duration_payload = {}
request_increases = {}
error_increases = {}
request_rates = {}
error_rates = {}
active_rates = {}
durations = {}

def supported_api_results_ready():
    return (
        all(request_increases.get(edge) == 40 for edge in expected_edges)
        and all(error_increases.get(edge) == 10 for edge in expected_edges)
        and set(request_rates) == expected_edges
        and set(error_rates) == expected_edges
        and all(abs(request_rates[edge] - 40/60) <= 0.02 for edge in expected_edges)
        and all(abs(error_rates[edge] - 10/60) <= 0.02 for edge in expected_edges)
        and set(active_rates) == expected_edges
        and all(math.isfinite(value) and value > 0 for value in active_rates.values())
        and set(durations) == expected_edges
        and all(math.isfinite(float(sample["value"])) and 0 < float(sample["value"]) < 5_000_000_000 for sample in durations.values())
    )

while time.time() < api_deadline:
    request_increase_payload = api_payload("servicegraph.request.count", "increase")
    error_increase_payload = api_payload("servicegraph.request.error.count", "increase")
    request_increases = latest_aggregate_by_edge(request_increase_payload["samples"])
    error_increases = latest_aggregate_by_edge(error_increase_payload["samples"])
    default_requests_payload = api_payload("servicegraph.request.count")
    error_rate_payload = api_payload("servicegraph.request.error.count", "window_rate")
    active_rate_payload = api_payload("servicegraph.request.count", "active_rate")
    duration_payload = api_payload("servicegraph.request.duration")
    request_rates = latest_aggregate_by_edge(default_requests_payload["samples"])
    error_rates = latest_aggregate_by_edge(error_rate_payload["samples"])
    active_rates = latest_aggregate_by_edge(active_rate_payload["samples"])
    durations = latest_by_edge(duration_payload["samples"])
    if supported_api_results_ready():
        break
    time.sleep(2)

assert all(request_increases.get(edge) == 40 for edge in expected_edges), request_increases
assert all(error_increases.get(edge) == 10 for edge in expected_edges), error_increases
assert all(sample["unit"] == "count" for sample in request_increase_payload["samples"])
assert_evaluated_query(request_increase_payload, "increase", "1m")
assert_evaluated_query(error_increase_payload, "increase", "1m")

assert_evaluated_query(default_requests_payload, "window_rate", "1m")
assert set(request_rates) == expected_edges, f"request rate missing internal edges: {request_rates.keys()}"
assert set(error_rates) == expected_edges, f"error rate missing internal edges: {error_rates.keys()}"
for edge in sorted(expected_edges):
    request_rate = request_rates[edge]
    error_rate = error_rates[edge]
    assert abs(request_rate - 40/60) <= 0.02, f"window request rate for {edge}: {request_rate} != 0.667"
assert abs(error_rate - 10/60) <= 0.02, f"window error rate for {edge}: {error_rate} != 0.167"
assert all(sample["unit"] == "per_second" for sample in default_requests_payload["samples"])

assert set(active_rates) == expected_edges, f"active rate missing cumulative servicegraph edges: {active_rates}"
assert all(math.isfinite(value) and value > 0 for value in active_rates.values())
print("✓ API exact increases, default/explicit window rates, active rates, metadata, and units are correct")

assert set(durations) == expected_edges, f"duration missing internal edges: {durations.keys()}"
for edge, sample in durations.items():
    value = float(sample["value"])
    assert math.isfinite(value) and 0 < value < 5_000_000_000, f"implausible duration for {edge}: {value}ns"
print("✓ servicegraph.request.duration is finite and plausible on all internal edges")
PY

echo "✓ E2E prometheus scenario passed"
