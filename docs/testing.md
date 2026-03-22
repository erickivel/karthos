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

The current e2e architecture is intentionally still:

- `kind` for the cluster
- `test/e2e/apps` for demo workloads
- `Taskfile.test.yml` for orchestration
- `test/e2e/scenarios/*/run.sh` for scenario validation

This slice does not replace the e2e architecture with KUTTL or Helm tests.

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

What each stage now does:

- `test:e2e:discover`
  writes `bin/e2e/profile.json`
- `test:e2e:plan`
  writes `bin/e2e/otel-values.yaml` and optional rendered chart output under `bin/e2e/rendered`
- `test:e2e:apply-otel`
  runs `helm upgrade --install` for agent, gateway, and backend
- `test:e2e:verify`
  checks Helm status, pod readiness, and recent agent/gateway/backend logs
- generated values can also carry collector `featureGates`; the default agent e2e flow enables `receiver.kubeletstats.enableCPUUsageMetrics` to reduce known kubeletstats deprecation noise

## Scenario Expectations

### Prometheus Available

`./test/e2e/scenarios/prometheus-available/run.sh`

Expected:

- Prometheus-annotated services are detected
- gateway-go emits OTLP traces (instrumented with OTel Go SDK, `peer.service` attributes set)
- generated Helm values include Prometheus receiver config and spanmetrics/servicegraph connectors
- agent DaemonSet, gateway Deployment, and backend Deployment become ready
- backend logs show accepted metric envelopes after the gateway forwards traffic
- after ~60s, `/debug/metrics` returns at least one metric sample
- `/debug/graph` returns a snapshot; when traces have flowed, edges include `calls` relations

Use the debug tasks to inspect backend state after a scenario run:

```bash
task test:e2e:debug:metrics  # dump all stored metric samples
task test:e2e:debug:graph    # dump full topology graph snapshot
```

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
task test:e2e:reset
task test:e2e:teardown
```
