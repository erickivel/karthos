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
- source-plan resolution, generic collection composition, and OTel adapter rendering (including Prometheus plus trace intake)
- Helm values generation
- backend OTLP envelope conversion and routing
- OTLP gRPC ingest integration
- `MetricTopologyBuilder`: observed `runs_on` edge extraction from resource attributes
- Pod lifecycle validation: UID/node matching, synchronous immutable `terminal_deadline` persistence before grace activation, fail-closed terminal refreshes on an initial marker-write failure with retry of the original deadline, confirmed-initial versus retry-only marker provenance (an absent/replaced retry-only UID retires immediately at restart), atomic active→pending→retired storage transitions that prevent delayed marker resurrection across delete/replace/crash/restart interleavings, cluster-scoped startup reconciliation and re-arming across same-UID/deleted/replaced restart states, deadline exclusion from current graph reads, immediate non-terminal deletion/replacement retirement, retry-until-cancellation persistence with bounded per-attempt backoff, and retained-edge `last_seen` stability
- `MultiIngestor`: fan-out behavior and error aggregation
- storage/query parity: append-only structural writes, 24-hour retained
  materialized graphs, current observed-edge merge rules, point-in-time
  selection, defensive memory-store copies,
  and 15-second evaluated metric ranges, including exact non-aligned final
  evaluations and explicit five-minute freshness behavior in VictoriaMetrics
- `/api/v1/graph` public graph-contract correctness
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
- `test:e2e:prepare-platform-images`
  builds and publishes backend/frontend images through the active cell's image
  transport; fixture deployment never builds those platform images
- `test:e2e:apply-otel`
  runs `discovery-job --mode=deploy`, which self-contains planning, compiles dynamic values, and uses
  Helm to deploy agent, gateway, backend, and frontend with the chart-owned
  source-contract ConfigMap
- `test:e2e:verify`
  checks Helm status, pod readiness including the frontend rollout, and recent agent/gateway/backend logs

`test:e2e:deploy` builds and publishes fixture images only, then waits for the
demo app Deployments and pods before discovery
runs. This keeps capability profiles from capturing terminating rollout pods
that would later appear as duplicate topology cards.

`test:e2e:plan` remains available only to inspect generated values or render
output manually. Live scenarios intentionally use the single deploy path:
fixture deployment → discovery → `e2e:apply-otel` → verification. The latter
first runs `e2e:prepare-platform-images`, so the deployed release never relies
on fixture deployment to build a platform image.

## Scenario Expectations

### Strict source-contract checks

`task test:e2e:scenario:detector-failure` proves discovery fails closed and
writes no capability profile when a required Kubernetes discovery-inventory
list cannot run.
`task test:e2e:scenario:source-contract-rejection` temporarily mutates the
Helm-owned contract only as a fault injection, verifies backend startup
rejection, then restores the valid ConfigMap and rollout automatically.
It waits for the Deployment controller to observe each restart generation
before judging rollout status, so an old healthy ReplicaSet cannot mask the
new contract result.

### Prometheus Available

`./test/e2e/scenarios/prometheus-available/run.sh`

This scenario covers Prometheus discovery, deploy, controlled traffic, and live
VictoriaMetrics/API assertions. The `e2e:scenario:prometheus` task invokes the
standalone runner, warms the cumulative servicegraph baseline, sends 30 normal
and 10 intentional `/error` requests, and verifies the resulting increases,
window rates, active rates, units, metadata, and duration.

Expected:

- Prometheus-annotated services are detected
- gateway-go emits OTLP traces (instrumented with OTel Go SDK, `peer.service` attributes set)
- generated Helm values include Prometheus receiver config and the selected gateway `service_graph` component ID
- agent DaemonSet, gateway Deployment, backend Deployment, and frontend Deployment become ready
- backend logs show accepted metric envelopes after the gateway forwards traffic
- generated traffic flows through `gateway-go -> service-a-python` and `gateway-go -> service-b-node`; the error fixture uses `gateway-go /error`, passes `force_error=true` downstream, returns 5xx responses, and marks the corresponding spans as error status
- after traffic has flowed, `/api/v1/metrics` returns normalized samples and edge-filtered queries can resolve live call edges
- `/api/v1/metrics` returns semconv servicegraph samples for `servicegraph.request.count`, `servicegraph.request.duration`, and `servicegraph.request.error.count`
- servicegraph samples preserve `failed`, `connection_type`, and distinct `client_http_method` / `server_http_method` labels when emitted; duration `_sum` and `_count` carry identical labels
- raw canonical servicegraph cumulative counters increase by exactly 40 requests and 10 errors for each of the three internal edges during the controlled traffic window; the scenario captures before/after VM baselines because collector v0.156 emits cumulative servicegraph Sums
- after a mature baseline, the runner waits until every expected edge has its complete supported API result set (explicit increases, default and explicit one-minute window rates, active rate, and duration); it then verifies `40/10` increases, approximately `0.667/0.167` rates per second, and matching response metadata/units. Raw-counter readiness alone is not sufficient because derived API results can arrive later.
- the controlled 40-request run can legitimately add 40 `otelcol_connector_servicegraph_expired_edges`: root inbound gateway server spans have no in-cluster client span to pair with. Internal pairing correctness is determined from the exact three internal edge totals, not from a zero global-expiration expectation; the generated connector pairing TTL remains 30 seconds
- `/api/v1/graph` returns the newest materialized snapshot. The runner polls
  through at most one five-minute materialization interval before requiring the
  controlled `calls` relations; it does not treat current collector state as a
  public graph response.
- Traffic topology mode intentionally stays aggregated at service/workload readability even when the granularity control is set to runtime or detailed; pod replica cards are not expected in Traffic mode

### Istio Mesh Available

`task test:e2e:scenario:istio`

Expected:

- the task installs a minimal Istio control plane if needed, deploys the `with-istio` fixture, generates normal plus intentional error gateway traffic, and applies the Karthos collector plan
- `karthos-demo` has sidecar injection enabled, while the sparse orphan pod explicitly disables injection
- discovery emits an `istio` source with `mesh.flavor=istio`, `mesh.mode=sidecar`, `mesh.partial=false`, and `/stats/prometheus` on port `15020`; complete sidecar meshes may use either a classic regular `istio-proxy` container or a Kubernetes native `initContainer` with `restartPolicy: Always`
- ambient mode remains unsupported because it has no per-Pod proxy scrape contract
- generated Helm values include a dedicated 15-second `mesh-istio` Prometheus scrape job that selects `istio-proxy` containers and stamps Prometheus-safe `karthos_source=mesh.istio`; the cadence keeps multiple cumulative samples inside the default one-minute aggregation window
- generated Helm values place discovered application Prometheus Service-DNS jobs on the singleton gateway and stamp them with `karthos_source=prometheus.app`; agent Prometheus remains reserved for Istio pod discovery
- application Prometheus annotations are not required for mesh metrics
- mesh-derived samples normalize to servicegraph edge metrics: HTTP request count/duration/error plus gRPC request/response messages and TCP opened/closed/sent/received counters
- the next materialized graph includes fresh mesh `calls` edges with
  `mesh.istio` provenance. They retain producer-time `last_seen` and mesh
  metadata such as `network.protocol.name`, `mesh.security.policy`, and peer
  SPIFFE principals; the runner waits at most one five-minute materialization
  interval before asserting them.
- an injected Go fixture warms the protocol counters, waits 180 seconds so the collector can capture a mature baseline and the warm-up leaves the one-minute window, then emits exactly 20 unary calls plus five bidirectional request/response messages and ten fresh 1,024-byte TCP echo connections
- destination-sidecar raw counters and canonical VictoriaMetrics counters increase by `25/25` gRPC messages, `10/10` TCP opened/closed connections, and `10,240/10,240` TCP sent/received bytes
- API `increase` values and base units match those totals; default one-minute rates are approximately `0.417 per_second`, `0.167 per_second`, and `170.667 bytes_per_second`; active rate is present for the cumulative series
- this remains an explicit cluster accuracy task and is not a GitHub Actions cluster job

### Host OS accuracy

The agent always uses `hostNetwork: true` with
`dnsPolicy: ClusterFirstWithHostNet`, so the hostmetrics network scraper observes
node interfaces rather than pod-network interfaces. Live host verification is a
synchronized comparison of `/proc/stat`, `/proc/diskstats`, `/proc/net/dev`,
filesystem truth, raw OTLP, VictoriaMetrics, and API results. No
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

Use the API shortcut to inspect backend state after traffic has flowed:

```bash
task test:e2e:debug:api      # query backend /api/v1/status on a dedicated local port
```

Recommended local port convention during e2e work:

- `18080` -> `gateway-go` traffic generation
- `18081` -> backend REST API
- `13133` -> backend health checks

The debug and traffic tasks never terminate a process occupying one of these
ports. They fail with the conflicting port and the Task variable to override,
for example `task test:e2e:traffic GATEWAY_LOCAL_PORT=18082`.

## Frontend Topology QA

The topology Playwright suite runs against the live frontend and a
port-forwarded backend API. Keep all Node/Playwright commands inside Docker.

Prerequisites:

- `task test:e2e:scenario:prometheus`
- `task web:e2e:install`
- `task web:dev`
- `kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080`

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
traffic cards, node CPU and memory are valid on Kubernetes node cards, and PVC
storage should only be asserted when the backend exposes paired PVC-resolved
volume capacity and available samples.

### No Telemetry Fallback

`./test/e2e/scenarios/no-telemetry/run.sh`

Expected:

- preparation first removes every E2E-owned workload and, if present, the
  E2E-owned Istio control plane; it refuses to remove an externally owned
  Istio installation and explains how to use a clean cluster
- `kubernetes.structure`, `kubelet`, `host-os`, and `kube-cluster` are
  available; Prometheus, Istio, and servicegraph are unavailable
- no Prometheus targets are present and the resolved source plan selects only
  `kubelet`, `host-os`, and `kube-cluster`
- generated values contain the direct collector requirements (`kubeletstats`,
  `hostmetrics`, and `k8s_cluster`), and the full collection plane deploys

## Logs and Failure Inspection

The backend uses structured JSON logs to stdout. The OTel adapter fixes agent
and gateway self-telemetry to JSON at `info` on stdout. Development-only
Collector debug-exporter output is transient diagnostics rather than structured
application logging. For operational debugging:

```bash
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=agent --tail=30
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=gateway --tail=30
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=backend --tail=30
helm status karthos -n karthos-system
```

When changing the e2e flow, keep log inspection and rendered Helm artifacts easy to collect from `bin/e2e/`.

## Cleanup

```bash
task test:e2e:reset       # remove the Helm-owned OTel release and its E2E PostgreSQL PVC
task test:e2e:reset-apps  # remove demo fixtures from karthos-demo and karthos-sparse
task test:e2e:reset-all   # remove release, fixtures, and E2E-owned Istio
task test:e2e:teardown    # destroy the kind cluster and local registry
```

The live harness always deploys the bundled PostgreSQL store. Reset removes
only that release-scoped E2E PVC so each scenario starts with the required
fresh materialized-snapshot schema; it does not target a remote PostgreSQL
deployment.
