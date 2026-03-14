# Testing Guide

This document covers the testing infrastructure for OKSG, including unit tests and end-to-end (e2e) testing with kind.

## Unit Tests

Run all unit tests:

```bash
task test
# or
go test ./...
```

### Key Test Files

| Package | Test File | Coverage |
|---------|-----------|----------|
| `pkg/adapters/kubernetes` | `discovery_test.go` | Discovery adapter with fake K8s client |
| `pkg/adapters/otel` | `source_planner_test.go` | OTel config generation |
| `pkg/collector/configgen` | `generator_test.go` | Manifest rendering |
| `pkg/collector/deployer` | `deployer_test.go` | kubectl apply mock |
| `cmd/discovery-job` | `main_test.go` | CLI modes with mocked dependencies |

## E2E Testing with kind

The e2e test infrastructure uses [kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker) to create a local cluster for integration testing.

### Prerequisites

- Docker
- kind
- kubectl

### Test Environment Setup

```bash
# Create the kind cluster with local registry
task test:e2e:setup
# or
./test/e2e/kind/setup.sh

# Verify cluster is running
kubectl cluster-info --context kind-oksg-test
```

### Application Variants

Two microservice variants are provided:

#### Base (No Telemetry)

Simple HTTP services without any observability instrumentation:

```
test/e2e/apps/base/
├── gateway-go/        # Go HTTP gateway (port 8080)
├── service-a-python/  # Python Flask service (port 8081)
├── service-b-node/    # Node.js Express service (port 8082)
├── namespace.yaml
├── gateway.yaml
├── service-a.yaml
├── service-b.yaml
└── kustomization.yaml
```

#### With Prometheus

Same services with Prometheus metrics endpoints:

```
test/e2e/apps/with-prometheus/
├── gateway-go/        # + /metrics endpoint, request counters/histograms
├── service-a-python/  # + prometheus_client metrics
├── service-b-node/    # + prom-client metrics
├── namespace.yaml
├── gateway.yaml       # + prometheus.io/scrape annotations
├── service-a.yaml     # + prometheus.io/scrape annotations
├── service-b.yaml     # + prometheus.io/scrape annotations
└── kustomization.yaml
```

### Deploying Applications

```bash
# Deploy base variant (no telemetry)
task test:e2e:deploy VARIANT=base
# or
./test/e2e/apps/deploy.sh base

# Deploy Prometheus-enabled variant
task test:e2e:deploy VARIANT=with-prometheus
# or
./test/e2e/apps/deploy.sh with-prometheus
```

### Running Discovery

```bash
# Run discovery against the cluster
task test:e2e:discover

# Generate OTel collector manifests
task test:e2e:plan

# Apply collectors to cluster
task test:e2e:apply-otel

# Verify collector state
task test:e2e:verify
```

### Test Scenarios

#### Prometheus Available Scenario

Tests that discovery correctly identifies Prometheus endpoints:

```bash
./test/e2e/scenarios/prometheus-available/run.sh
```

**Expected behavior:**
- Discovers services with `prometheus.io/scrape: "true"` annotations
- Generates OTel config with `prometheus` receiver
- Creates scrape configs for each discovered target

#### No Telemetry Fallback Scenario

Tests the fallback when no telemetry sources are found:

```bash
./test/e2e/scenarios/no-telemetry/run.sh
```

**Expected behavior:**
- No Prometheus targets discovered
- Generates OTel config with only `kubeletstats` receiver
- System remains functional for infrastructure metrics

### Cleanup

```bash
# Remove OTel collectors only
task test:e2e:reset

# Tear down entire cluster
task test:e2e:teardown
# or
./test/e2e/kind/teardown.sh
```

## Test Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      kind Cluster (oksg-test)                    │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    oksg-demo namespace                      │ │
│  │                                                             │ │
│  │  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │ │
│  │  │  gateway-go  │───▶│service-a-py  │───▶│service-b-node│  │ │
│  │  │   :8080      │    │   :8081      │    │    :8082     │  │ │
│  │  │  [/metrics]  │    │  [/metrics]  │    │  [/metrics]  │  │ │
│  │  └──────────────┘    └──────────────┘    └──────────────┘  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                   oksg-system namespace                     │ │
│  │                                                             │ │
│  │  ┌──────────────┐              ┌──────────────┐            │ │
│  │  │ OTel Agent   │─────OTLP────▶│ OTel Gateway │            │ │
│  │  │ (DaemonSet)  │              │ (Deployment) │            │ │
│  │  └──────────────┘              └──────────────┘            │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Local Registry                           │ │
│  │                 localhost:5001                              │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Taskfile Commands Reference

All e2e tasks are defined in `Taskfile.test.yml`:

| Task | Description |
|------|-------------|
| `test:e2e:setup` | Create kind cluster and local registry |
| `test:e2e:teardown` | Delete kind cluster and registry |
| `test:e2e:deploy` | Build and deploy app variant |
| `test:e2e:discover` | Run discovery job |
| `test:e2e:plan` | Generate OTel manifests |
| `test:e2e:apply-otel` | Apply OTel collectors |
| `test:e2e:verify` | Check collector status |
| `test:e2e:reset` | Remove OTel collectors |
| `test:e2e:scenario:prometheus` | Run Prometheus scenario |
| `test:e2e:scenario:fallback` | Run no-telemetry scenario |
