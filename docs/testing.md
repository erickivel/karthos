# Testing Guide

This document covers unit tests plus the current end-to-end flow built on `kind`, the existing demo apps, and shell-driven scenarios.

## Unit Tests

Run unit tests with:

```bash
task test
```

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
task test:e2e:deploy VARIANT=with-prometheus
task test:e2e:discover
task test:e2e:plan
task test:e2e:apply-otel
task test:e2e:verify
```

`task test:e2e:setup` provisions one control-plane node and one worker
node so topology QA can validate cross-node placement in the live cluster.

What each stage does:

- `test:e2e:discover`
  writes `bin/e2e/profile.json`
- `test:e2e:plan`
  writes `bin/e2e/otel-values.yaml` and optional rendered chart output under `bin/e2e/rendered`
- `test:e2e:apply-otel`
  runs `helm upgrade --install` for agent, gateway, backend, and frontend
- `test:e2e:verify`
  checks Helm status, pod readiness including the frontend rollout, and recent agent/gateway/backend logs
- generated values can also carry collector `featureGates`; the default agent e2e flow enables `receiver.kubeletstats.enableCPUUsageMetrics` to reduce known kubeletstats deprecation noise

`test:e2e:deploy` waits for the demo app Deployments and pods before discovery
runs. This keeps discovery profiles from capturing terminating rollout pods
that would later appear as duplicate topology cards.

## Scenario Expectations

### Prometheus Available

`./test/e2e/scenarios/prometheus-available/run.sh`

This scenario covers Prometheus discovery, deploy, and traffic generation. The `e2e:scenario:prometheus` task runs the deploy pipeline and then sends gateway traffic automatically.

Expected:

- Prometheus-annotated services are detected
- gateway-go emits OTLP traces (instrumented with OTel Go SDK, `peer.service` attributes set)
- generated Helm values include Prometheus receiver config and the gateway `spanmetrics`/`servicegraph` connectors
- agent DaemonSet, gateway Deployment, backend Deployment, and frontend Deployment become ready
- backend logs show accepted metric envelopes after the gateway forwards traffic
- generated traffic flows through `gateway-go -> service-a-python` and `gateway-go -> service-b-node`
- after traffic has flowed, `/api/v1/metrics` returns normalized samples and edge-filtered queries can resolve live call edges
- `/api/v1/metrics` returns semconv servicegraph samples for `servicegraph.request.count`, `servicegraph.request.duration`, and `servicegraph.request.error.count`
- `servicegraph.request.error.ratio` remains query-derived and is safe to request through the same API
- `/debug/graph` returns a snapshot; when traces have flowed, edges include `calls` relations
- Traffic topology mode intentionally stays aggregated at service/workload readability even when the granularity control is set to runtime or detailed; pod replica cards are not expected in Traffic mode

### Istio Mesh Available

`./test/e2e/scenarios/istio-available/run.sh`

Expected:

- `task test:e2e:scenario:istio` installs a minimal Istio control plane if needed, deploys the `with-istio` fixture, generates gateway traffic, and applies the OKSG collector plan
- `oksg-demo` has sidecar injection enabled, while the sparse orphan pod explicitly disables injection
- discovery emits an `istio` source with `mesh.flavor=istio`, `mesh.mode=sidecar`, and `/stats/prometheus` on port `15020`
- generated Helm values include a dedicated `mesh-istio` Prometheus scrape job that selects `istio-proxy` containers and stamps Prometheus-safe `oksg_source=mesh.istio`
- generated Helm values stamp discovered application Prometheus scrape jobs with `oksg_source=prometheus.app`
- application Prometheus annotations are not required for mesh metrics
- mesh-derived samples normalize to existing servicegraph edge metrics, including `servicegraph.request.count`, `servicegraph.request.duration`, and `servicegraph.request.error.count`
- graph `calls` edges include `mesh.istio` provenance, advance `last_seen` from metric datapoint timestamps, and preserve mesh metadata such as `network.protocol.name`, `mesh.security.policy`, and peer SPIFFE principals

## Generating Traffic

To produce observable telemetry after a scenario is running, use the traffic tasks. They port-forward to `gateway-go` and send HTTP requests through the full call chain (`gateway-go → service-a-python → service-b-node`).

Send a fixed number of requests (default 20, 0.5 s apart):

```bash
task test:e2e:traffic
# override defaults:
task test:e2e:traffic TRAFFIC_REQUESTS=50 TRAFFIC_INTERVAL=0.2
```

Stream requests continuously until Ctrl-C:

```bash
task test:e2e:traffic:watch
task test:e2e:traffic:watch TRAFFIC_INTERVAL=1
```

`task test:e2e:scenario:prometheus` already runs gateway traffic as the final step of the scenario. Use the standalone traffic tasks to generate additional load after the scenario has deployed.

Typical sequence after a scenario run:

```bash
task test:e2e:scenario:prometheus   # deploy, wire up telemetry, and send initial gateway traffic
task test:e2e:traffic TRAFFIC_REQUESTS=30  # optional: send more traffic
task test:e2e:debug:api             # verify backend REST API access
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count" | jq .
task test:e2e:debug:graph           # inspect topology graph snapshot
```

Configurable variables (all override-able on the command line):

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_LOCAL_PORT` | `18080` | Local port used for the gateway port-forward |
| `BACKEND_API_LOCAL_PORT` | `18081` | Local port used for the backend REST API port-forward |
| `TRAFFIC_REQUESTS` | `20` | Number of requests sent by `e2e:traffic` |
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
- generated Helm values keep only `kubeletstats`
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
