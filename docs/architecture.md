# Architecture Overview

OKSG transforms discovered observability signals into a canonical graph-oriented foundation. The currently implemented slice now includes a discovery-driven OpenTelemetry collection plane plus the first backend OTLP ingest service.

## Current Implemented Boundary

The implemented control flow is:

```text
Kubernetes API -> ClusterProfile -> SourcePlanner -> Helm Values -> Agent/Gateway/Backend Helm Release
```

What is real today:

- `pkg/adapters/kubernetes` discovers cluster resources and Prometheus annotation-based targets
- `pkg/adapters/otel` plans collector behavior from the discovery profile
- `pkg/adapters/otlp` exposes OTLP gRPC and HTTP ingest for the backend runtime
- `pkg/collector/configgen` converts the plan into typed Helm values
- `pkg/collector/deployer` runs Helm lint/template/upgrade-install
- `pkg/backend/ingest` converts OTLP telemetry into raw envelopes and routes them by signal type
- `cmd/backend` runs the first backend OTLP ingest service
- `cmd/discovery-job` orchestrates discover, plan, and deploy modes

What is not implemented yet:

- `cmd/controller` is still a placeholder
- persistence, query APIs, graph assembly, and canonical normalization still remain the next major runtime layers

## Layer View

```text
Cluster Integration
  Kubernetes discovery adapter

Collection Planning
  Source planner
  Collector config assembly

Collection Runtime
  Agent collector
  Gateway collector
  Backend OTLP ingest service

Deployment Packaging
  Helm values builder
  Helm chart
  Helm release installer

Future Layers
  semantic processing
  persistence
  query and control APIs
  modeling and prediction
```

## Deployment Model

The Helm chart in `deploy/helm/oksg-otel` owns the Kubernetes resource shape:

- namespace creation
- agent ConfigMap
- gateway ConfigMap
- agent ServiceAccount and RBAC
- agent DaemonSet
- gateway ServiceAccount
- gateway Service
- gateway Deployment
- backend Service
- backend Deployment

The planner no longer renders Kubernetes objects directly. It produces collector configuration that is embedded into release values, and Helm renders the final resources.

## Configuration Model

Configuration is split intentionally:

- Helm values:
  operator-facing deployment settings such as release name, image settings, namespace, enabled signal classes, OTLP endpoints, and log levels
- ConfigMaps:
  structured collector configuration for agent and gateway
- Secrets:
  reserved for credentials and sensitive settings
- Env vars:
  runtime toggles and pod or node metadata only

## Logging Policy

OKSG-owned runtime components use structured JSON logs to stdout only.

- required fields: `ts`, `level`, `component`, `msg`
- common context when available: `cluster`, `namespace`, `mode`, `error`
- debugging path: `kubectl logs`

OTel collector workloads are also configured for stdout-based logging, with per-component log level controlled through Helm values.

The backend OTLP ingest runtime logs one structured line per accepted envelope so end-to-end verification can use `kubectl logs` before persistence exists.

## Design Principles Still In Force

1. Keep the design minimal and easy to change.
2. Preserve explicit contracts between discovery, planning, deployment, and future graph layers.
3. Let discovery decide what to collect, not how Kubernetes YAML is shaped.
4. Keep technology-specific adapters isolated from domain packages.
5. Remove stale deployment paths and outdated docs when replacing them.
