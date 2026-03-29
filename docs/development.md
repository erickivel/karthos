# Development Guide

This guide focuses on the current implementation shape rather than future-only layouts.

## Prerequisites

- Go 1.26+
- [Task](https://taskfile.dev/)
- Docker
- kind
- kubectl
- helm

## Tech Stack

Keep the main application minimal:

- Go
- Kubernetes Go client for discovery and cluster-side deployment workflows
- `sigs.k8s.io/yaml` for collector config and Helm values rendering
- Prometheus packages only when a concrete Prometheus adapter, parser, or test application needs them
- OTLP packages only when an OTLP adapter or receiver is added
- `github.com/google/uuid` for event identifiers
- `github.com/invopop/jsonschema` for generated schema documentation
- `github.com/jackc/pgx/v5` for PostgreSQL persistence (pure Go, zero CGO)
- `github.com/stretchr/testify` for tests

Do not add large framework dependencies before there is a concrete need for them. See [GUIDELINE.md](../GUIDELINE.md) for the full dependency philosophy.

## Current Structure

```text
cmd/
  discovery-job/   implemented discovery/plan/deploy CLI
  backend/         OTLP ingest and semantic normalization runtime
  controller/      placeholder
  schema-gen/      schema helper

pkg/
  adapters/kubernetes/   discovery adapter (Prometheus + OTLP workload detection)
  adapters/otel/         source planner (hostmetrics, k8s_cluster, spanmetrics, servicegraph)
  adapters/otlp/         OTLP gRPC and HTTP server adapter
  backend/ingest/        raw-envelope conversion, signal routing, entity resolution,
                         OTLP normalization, topology building, and multi-ingestor fan-out
                         (entity_resolver.go, normalizer.go, topology_builder.go,
                          metric_topology_builder.go, normalizing_ingestor.go,
                          plus existing otlp.go, ingest.go, semantic.go)
  backend/persistence/   store interfaces (MetricStore, GraphStore, MetadataStore,
                         RawStore, EntityRegistry) and in-memory implementations
  backend/query/         graph and metrics query contracts
  adapters/storage/      storage factory and config (memory or external backends)
  adapters/storage/postgresql/     PostgreSQL implementations (pgx/v5)
  adapters/storage/victoriametrics/ VictoriaMetrics MetricStore (HTTP API)
  collector/configgen/   Helm values builder
  collector/deployer/    Helm runner
  core/logging/          shared JSON stdout logger
  discovery/profile/     ClusterProfile types

  core/
    entities/        canonical graph-facing entities and stable IDs
    metrics/         canonical metric definitions, units, and samples
    topology/        graph nodes, edges, relations, and snapshots
    registry/        canonical metric registry contracts
    featurestore/    reserved for future model-ready features
    schema/          raw telemetry envelopes, validation, schema versions

  modeling/
    scenarios/       reserved scenario definitions
    predictors/      predictor interfaces only
    simulation/      reserved simulation boundary

deploy/
  helm/              Helm charts and release packaging
  manifests/         raw manifest placeholder
  examples/          example deployment placeholder

test/
  e2e/
    kind/            kind cluster setup scripts and config
    apps/            example microservices for base and Prometheus scenarios
    scenarios/       end-to-end validation scripts and expected outputs
```

## Common Commands

```bash
task fmt
task test
task build
task vet
task lint
task ci
```

## Discovery And Deployment Workflow

```bash
./bin/discovery-job --mode=discover --kubeconfig=$HOME/.kube/config --cluster-name=oksg-test --output=profile.json
./bin/discovery-job --mode=plan --profile=profile.json --values-output=otel-values.yaml --rendered-output=./bin/e2e/rendered --backend-image-repository=localhost:5001/oksg-backend --backend-image-tag=latest
./bin/discovery-job --mode=deploy --profile=profile.json --release-name=oksg-otel --chart-path=./deploy/helm/oksg-otel --namespace=oksg-system --backend-image-repository=localhost:5001/oksg-backend --backend-image-tag=latest
./bin/backend --grpc-listen-address=0.0.0.0:4317 --http-listen-address=0.0.0.0:4318 --health-listen-address=0.0.0.0:13133 --enabled-signals=metric,trace --cluster-name=oksg-test
# With external storage:
./bin/backend --storage-backend=external --postgres-dsn="postgres://oksg:pass@localhost:5432/oksg" --victoriametrics-url="http://localhost:8428" --raw-store-enabled=true --enabled-signals=metric,trace --cluster-name=oksg-test
```

Notes:

- use file outputs for `discover` and `plan` when you need clean machine-readable artifacts
- `plan` now emits Helm values, not raw manifests
- `deploy` uses Helm, not `kubectl apply`
- collector runtime flags are driven from Helm values; `agent.featureGates` currently enables `receiver.kubeletstats.enableCPUUsageMetrics` to avoid deprecated kubelet CPU utilization metrics

## Helm Ownership

The chart owns Kubernetes resource rendering. When changing deployment behavior:

1. update planner output only when collection behavior changes
2. update values builder when deployment intent changes
3. update chart templates when Kubernetes resource shape changes
4. update `values.schema.json` whenever values shape changes
5. update `deploy/images/backend/Dockerfile` and e2e image build flow when backend runtime packaging changes

Do not reintroduce ad hoc manifest rendering in Go.

## Logging

OKSG-owned components log structured JSON to stdout only.

- configure level through `--log-level` or environment defaults
- inspect runtime behavior with `kubectl logs`
- keep operational logging in docs, not buried in ad hoc comments

## Maintenance Rule

When you replace a flow or interface:

- remove dead functions and retired compatibility code
- delete obsolete templates
- rewrite stale docs and task examples in the same change

## Debugging

```bash
helm lint ./deploy/helm/oksg-otel -f bin/e2e/otel-values.yaml
helm template oksg-otel ./deploy/helm/oksg-otel --namespace oksg-system -f bin/e2e/otel-values.yaml
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=agent --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=gateway --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=backend --tail=30
```

## Inspecting the Backend Store

The backend exposes debug endpoints on its health port (13133):

- `/debug/metrics` — all metric samples currently in the in-memory store as JSON
- `/debug/graph` — current topology snapshot including accumulated nodes and edges as JSON

Use the task shortcuts:

```bash
# Dump all stored metric samples (port-forwards, curls, then cleans up)
task test:e2e:debug:metrics

# Dump the current topology graph snapshot
task test:e2e:debug:graph

# Filter backend logs for write confirmations and skipped-entity warnings
task test:e2e:debug:logs
```

Or run the steps manually:

```bash
kubectl port-forward -n oksg-system deployment/oksg-backend 13133:13133 &
curl -s http://localhost:13133/debug/metrics | jq '.[0:10]'
curl -s http://localhost:13133/debug/graph | jq '.graph.edges[0:5]'
kill %1
```

### Reading the graph output

A graph snapshot from `/debug/graph` looks like:

```json
{
  "graph": {
    "nodes": [
      {"id": "...", "kind": "pod", ...}
    ],
    "edges": [
      {"source": "...", "target": "...", "relation": "runs_on", ...},
      {"source": "...", "target": "...", "relation": "calls", ...}
    ]
  }
}
```

Expected edge relations after telemetry flows:
- `runs_on`: pod → node (from kubeletstats resource attributes)
- `contains`: namespace → pod, pod → container (from kubeletstats resource attributes)
- `calls`: service → service (from OTLP trace `peer.service` attributes, when traces are enabled)

### Reading the output

A successful write line in backend logs looks like:
```json
{"level":"info","msg":"wrote metric samples","source":"otlp/grpc","count":3}
```

A skipped envelope (expected for k8s_cluster workload-object metrics and raw host metrics without node context) looks like:
```json
{"level":"warn","msg":"skipping envelope: unresolvable entity","source":"otlp/grpc","resource":{"k8s.deployment.name":"..."}}
```

Skips for `k8s.namespace`, `k8s.deployment`, `k8s.replicaset`, `k8s.daemonset` resources are expected — these entity kinds are not yet part of the canonical model. Skips with `"resource":null` indicate hostmetrics scrapers whose node context was not injected (check the `resource` processor and `K8S_NODE_NAME` env var in the agent DaemonSet).
