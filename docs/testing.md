# Testing Guide

This document covers unit tests plus the current end-to-end flow built on `kind`, the existing demo apps, and shell-driven scenarios.

## Unit Tests

Run unit tests with:

```bash
task test
```

`task ci` also runs `task test:fixtures`, covering both nested Go fixture
modules: `instrumented/gateway-go` and `mesh-protocol-go`.

Key coverage areas:

- Kubernetes discovery adapter behavior
- source planner receiver selection (including prometheus+traces combined profile)
- Helm values generation
- backend OTLP envelope conversion and routing
- OTLP gRPC and HTTP ingest integration
- `MetricTopologyBuilder`: structural edge and node extraction from resource attributes
- `MultiIngestor`: fan-out behavior and error aggregation
- `InMemoryGraphStore`: node accumulation and deduplication across snapshots
- `/debug/graph` endpoint correctness
- Helm deploy command assembly
- structured JSON logger behavior
- `discovery-job` mode orchestration

## E2E Model

- `kind` for the cluster
- `test/e2e/apps` for demo workloads
- `Taskfile.test.yml` for orchestration
- `test/e2e/scenarios/*/run.sh` for scenario validation

## Prerequisites

- Docker
- kind
- kubectl
- helm

## Main E2E Flow

```bash
task test:e2e:setup
VARIANT=with-prometheus task test:e2e:deploy
task test:e2e:discover
task test:e2e:plan
task test:e2e:apply-otel
task test:e2e:verify
```

`task test:e2e:setup` provisions one control-plane node and one worker
node so topology QA can validate cross-node placement in the live cluster.

> **`VARIANT` contract:** `e2e:deploy` reads `VARIANT` from the shell
> environment only. There is no Task-CLI-var form — the previous
> `task test:e2e:deploy VARIANT=with-prometheus` syntax does not
> propagate. Pass it as an env-var prefix on the CLI
> (`VARIANT=… task …`). Internal scenarios call `deploy.sh` with an explicit
> variant argument so scenario correctness does not depend on nested Task
> environment propagation.

The same `task test:e2e:*` commands work against a remote k3s cluster
reached over SSH — set `CLUSTER_PROFILE` in `.env` (see
[remote-cluster.md](remote-cluster.md)).

What each stage does:

- `test:e2e:discover`
  writes `bin/e2e/profile.json`
- `test:e2e:plan`
  writes `bin/e2e/otel-values.yaml` and optional rendered chart output under `bin/e2e/rendered`
- `test:e2e:apply-otel`
  runs `helm upgrade --install` for agent, gateway, backend, and frontend
- `test:e2e:verify`
  checks Helm status, pod readiness including the frontend rollout, and recent agent/gateway/backend logs
- generated values can also carry collector `featureGates`; v0.156 agent gates are empty and the gateway disables `connector.spanmetrics.includeCollectorInstanceID` for compatibility

`test:e2e:deploy` waits for the demo app Deployments and pods before discovery
runs. This keeps discovery profiles from capturing terminating rollout pods
that would later appear as duplicate topology cards.

## Scenario Expectations

### Prometheus Available

`./test/e2e/scenarios/prometheus-available/run.sh`

This scenario covers Prometheus discovery, deploy, controlled traffic, and live
VictoriaMetrics/API assertions. The `e2e:scenario:prometheus` task invokes the
standalone runner, warms the cumulative servicegraph baseline, sends 30 normal
and 10 intentional `/error` requests, and verifies the resulting increases,
window rates, active rates, units, metadata, duration, and error ratio.

Expected:

- Prometheus-annotated services are detected
- gateway-go emits OTLP traces (instrumented with OTel Go SDK, `peer.service` attributes set)
- generated Helm values include Prometheus receiver config and the gateway `span_metrics`/`service_graph` component IDs
- agent DaemonSet, gateway Deployment, backend Deployment, and frontend Deployment become ready
- backend logs show accepted metric envelopes after the gateway forwards traffic
- generated traffic flows through `gateway-go -> service-a-python` and `gateway-go -> service-b-node`; the error fixture uses `gateway-go /error`, passes `force_error=true` downstream, returns 5xx responses, and marks the corresponding spans as error status
- after traffic has flowed, `/api/v1/metrics` returns normalized samples and edge-filtered queries can resolve live call edges
- `/api/v1/metrics` returns semconv servicegraph samples for `servicegraph.request.count`, `servicegraph.request.duration`, and `servicegraph.request.error.count`
- servicegraph samples preserve `failed`, `connection_type`, and distinct `client_http_method` / `server_http_method` labels when emitted; duration `_sum` and `_count` carry identical labels
- raw canonical servicegraph cumulative counters increase by exactly 40 requests and 10 errors for each of the three internal edges during the controlled traffic window; the scenario captures before/after VM baselines because collector v0.156 emits cumulative servicegraph Sums
- after a mature baseline, explicit increases are `40/10`, default and explicit one-minute window rates are approximately `0.667/0.167` per second, cumulative active rate is available, response metadata/units match the selected mode, and every edge ratio is `0.25`
- the controlled 40-request run can legitimately add 40 `otelcol_connector_servicegraph_expired_edges`: root inbound gateway server spans have no in-cluster client span to pair with. Internal pairing correctness is determined from the exact three internal edge totals, not from a zero global-expiration expectation; the generated connector pairing TTL remains 30 seconds
- `servicegraph.request.error.ratio` remains query-derived, groups by edge plus request context while excluding `failed`, and must return `0.25 ± 0.02` for every controlled internal edge
- `/debug/graph` returns a snapshot; when traces have flowed, edges include `calls` relations
- Traffic topology mode intentionally stays aggregated at service/workload readability even when the granularity control is set to runtime or detailed; pod replica cards are not expected in Traffic mode

### Istio Mesh Available

`task test:e2e:scenario:istio`

Expected:

- the task installs a minimal Istio control plane if needed, deploys the `with-istio` fixture, generates normal plus intentional error gateway traffic, and applies the OKSG collector plan
- `oksg-demo` has sidecar injection enabled, while the sparse orphan pod explicitly disables injection
- discovery emits an `istio` source with `mesh.flavor=istio`, `mesh.mode=sidecar`, and `/stats/prometheus` on port `15020`
- generated Helm values include a dedicated 15-second `mesh-istio` Prometheus scrape job that selects `istio-proxy` containers and stamps Prometheus-safe `oksg_source=mesh.istio`; the cadence keeps multiple cumulative samples inside the default one-minute aggregation window
- generated Helm values place discovered application Prometheus Service-DNS jobs on the singleton gateway and stamp them with `oksg_source=prometheus.app`; agent Prometheus remains reserved for Istio pod discovery
- application Prometheus annotations are not required for mesh metrics
- mesh-derived samples normalize to existing servicegraph edge metrics, including `servicegraph.request.count`, `servicegraph.request.duration`, and `servicegraph.request.error.count`
- graph `calls` edges include `mesh.istio` provenance, advance `last_seen` from metric datapoint timestamps, and preserve mesh metadata such as `network.protocol.name`, `mesh.security.policy`, and peer SPIFFE principals
- an injected Go fixture warms the protocol counters, waits 180 seconds so the collector can capture a mature baseline and the warm-up leaves the one-minute window, then emits exactly 20 unary calls plus five bidirectional request/response messages and ten fresh 1,024-byte TCP echo connections
- destination-sidecar raw counters and canonical VictoriaMetrics counters increase by `25/25` gRPC messages, `10/10` TCP opened/closed connections, and `10,240/10,240` TCP sent/received bytes
- API `increase` values and base units match those totals; default one-minute rates are approximately `0.417 per_second`, `0.167 per_second`, and `170.667 bytes_per_second`; active rate is present for the cumulative series
- this remains an explicit cluster accuracy task and is not a GitHub Actions cluster job

### Host OS accuracy

Render the agent chart both with the default `agent.hostNetwork=true` and with
the opt-out. The default must use `dnsPolicy: ClusterFirstWithHostNet` and retain
the network scraper; opt-out must omit that scraper. Live host verification is a
synchronized comparison of `/proc/stat`, `/proc/diskstats`, `/proc/net/dev`,
`/proc/vmstat`, filesystem truth, raw OTLP, VictoriaMetrics, and API results. No
privileged host fixture or historical-series deletion code belongs in the repo.

## Generating Traffic

To produce observable telemetry after a scenario is running, use the traffic tasks. They port-forward to `gateway-go` and send HTTP requests through the full call chain (`gateway-go → service-a-python → service-b-node`). Error traffic calls `gateway-go /error`; the fixture propagates `force_error=true` to downstream services, returns 5xx responses, and marks spans with error status so servicegraph error metrics can be produced.

Send a fixed number of requests (default 20, 0.5 s apart):

```bash
task test:e2e:traffic
# override defaults:
task test:e2e:traffic TRAFFIC_REQUESTS=50 TRAFFIC_INTERVAL=0.2
# include intentional error requests for servicegraph.request.error.count:
task test:e2e:traffic TRAFFIC_ERROR_REQUESTS=10
```

Stream requests continuously until Ctrl-C:

```bash
task test:e2e:traffic:watch
task test:e2e:traffic:watch TRAFFIC_INTERVAL=1
task test:e2e:traffic:watch TRAFFIC_ERROR_EVERY=0 # disable /error requests
```

`task test:e2e:scenario:prometheus` already runs normal and error gateway
traffic and completes the live metric assertions. Use the standalone traffic
tasks to generate additional load after the scenario has deployed.

Typical sequence after a scenario run:

```bash
task test:e2e:scenario:prometheus   # deploy, generate controlled traffic, and assert VM/API semantics
task test:e2e:traffic TRAFFIC_REQUESTS=30  # optional: send more traffic
task test:e2e:traffic TRAFFIC_ERROR_REQUESTS=10  # optional: send more error traffic
task test:e2e:debug:api             # verify backend REST API access
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count" | jq .
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.error.count" | jq .
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.error.ratio" | jq .
task test:e2e:debug:graph           # inspect topology graph snapshot
```

Configurable variables (all override-able on the command line):

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_LOCAL_PORT` | `18080` | Local port used for the gateway port-forward |
| `BACKEND_API_LOCAL_PORT` | `18081` | Local port used for the backend REST API port-forward |
| `TRAFFIC_REQUESTS` | `20` | Number of requests sent by `e2e:traffic` |
| `TRAFFIC_ERROR_REQUESTS` | `0` | Number of intentional `/error` requests sent by `e2e:traffic`; scenarios override this to generate servicegraph error samples |
| `TRAFFIC_ERROR_EVERY` | `5` | In `e2e:traffic:watch`, every Nth request uses `/error`; set `0` to disable |
| `TRAFFIC_INTERVAL` | `0.5` | Seconds between requests |

Use the debug tasks to inspect backend state after traffic has flowed:

```bash
task test:e2e:debug:api      # query backend /api/v1/status on a dedicated local port
task test:e2e:debug:metrics  # memory-store only; external storage users should query /api/v1/metrics or VictoriaMetrics directly
task test:e2e:debug:graph    # dump full topology graph snapshot
```

Recommended local port convention during e2e work:

- `18080` -> `gateway-go` traffic generation
- `18081` -> backend REST API
- `13133` -> backend debug/health

## Frontend Topology QA

The topology Playwright suite runs against the live frontend and a
port-forwarded backend API. Keep all Node/Playwright commands inside Docker.

Prerequisites:

- `task test:e2e:scenario:prometheus`
- `task web:e2e:install`
- `task web:dev`
- `kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080`

Run the suite:

```bash
task web:e2e
```

Update committed screenshot baselines for the 9 projection/granularity views:

```bash
task web:e2e:update
```

Coverage currently includes:

- projection mode grouping behavior
- granularity transitions and selection persistence
- sparse/orphan pod handling
- infrastructure overview rendering Kubernetes nodes as metric cards
- compact infrastructure runtime pod packing inside node groups
- traffic cards rendering servicegraph request rates
- screenshot baselines for all `mode x granularity` combinations

Topology metric assertions should stay entity-scoped. Pod CPU and memory are
valid on pod cards, servicegraph request rates are valid on service and ingress
traffic cards, node allocatable CPU and memory are valid on Kubernetes node
cards, and workload readiness uses `k8s.deployment.available` over discovered
`k8s.workload.replicas` metadata. PVC storage should only be asserted when the
backend exposes paired PVC-resolved volume capacity and available samples.

### No Telemetry Fallback

`./test/e2e/scenarios/no-telemetry/run.sh`

Expected:

- no Prometheus targets are present
- generated Helm values keep only the `kubelet_stats` receiver instance
- agent, gateway, and backend still deploy successfully
- backend logs still show infrastructure-oriented envelopes received through the fallback path

## Logs and Failure Inspection

OKSG uses structured JSON logs to stdout only. For operational debugging:

```bash
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=agent --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=gateway --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=backend --tail=30
helm status oksg-otel -n oksg-system
```

When changing the e2e flow, keep log inspection and rendered Helm artifacts easy to collect from `bin/e2e/`.

## Cleanup

```bash
task test:e2e:reset       # remove OTel collectors and the applied discovery-profile ConfigMap
task test:e2e:reset-apps  # remove demo fixtures from oksg-demo and oksg-sparse
task test:e2e:reset-all   # remove both OTel collectors and all demo fixtures
task test:e2e:teardown    # destroy the kind cluster and local registry
```
