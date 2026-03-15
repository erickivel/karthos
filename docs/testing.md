# Testing Guide

This document covers unit tests plus the current end-to-end flow built on `kind`, the existing demo apps, and shell-driven scenarios.

## Unit Tests

Run unit tests with:

```bash
task test
```

Key coverage areas:

- Kubernetes discovery adapter behavior
- source planner receiver selection
- Helm values generation
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
  runs `helm upgrade --install`
- `test:e2e:verify`
  checks Helm status, pod readiness, and recent agent/gateway logs

## Scenario Expectations

### Prometheus Available

`./test/e2e/scenarios/prometheus-available/run.sh`

Expected:

- Prometheus-annotated services are detected
- generated Helm values include Prometheus receiver config
- agent DaemonSet and gateway Deployment become ready

### No Telemetry Fallback

`./test/e2e/scenarios/no-telemetry/run.sh`

Expected:

- no Prometheus targets are present
- generated Helm values keep only `kubeletstats`
- agent and gateway still deploy successfully

## Logs and Failure Inspection

OKSG uses structured JSON logs to stdout only. For operational debugging:

```bash
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=agent --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=gateway --tail=30
helm status oksg-otel -n oksg-system
```

When changing the e2e flow, keep log inspection and rendered Helm artifacts easy to collect from `bin/e2e/`.

## Cleanup

```bash
task test:e2e:reset
task test:e2e:teardown
```
