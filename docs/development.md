# Development Guide

This document provides guidance for developing and extending OKSG.

## Prerequisites

- Go 1.23+
- [Task](https://taskfile.dev/) (task runner)
- Docker (for e2e tests)
- kind (for e2e tests)
- kubectl (for e2e tests)
- golangci-lint (optional, for linting)

## Project Structure

```
.
├── cmd/
│   ├── backend/         # Backend API entrypoint (future)
│   ├── controller/      # K8s controller entrypoint (future)
│   ├── discovery-job/   # Discovery CLI (implemented)
│   └── schema-gen/      # JSON Schema generator
├── pkg/
│   ├── adapters/
│   │   ├── kubernetes/  # K8s discovery adapter
│   │   ├── otel/        # OTel source planner + templates
│   │   ├── prometheus/  # Prometheus adapter (future)
│   │   ├── istio/       # Istio adapter (future)
│   │   └── otlp/        # OTLP adapter (future)
│   ├── backend/
│   │   ├── ingest/      # Ingestion contracts
│   │   ├── query/       # Query contracts
│   │   └── persistence/ # Store interfaces
│   ├── collector/
│   │   ├── configgen/   # Manifest generator
│   │   ├── deployer/    # kubectl deployer
│   │   ├── pipelines/   # Pipeline contracts
│   │   └── exporters/   # Exporter contracts (future)
│   ├── core/
│   │   ├── entities/    # Canonical entities
│   │   ├── metrics/     # Metric definitions
│   │   ├── topology/    # Graph structures
│   │   ├── registry/    # Metric registry
│   │   └── schema/      # Telemetry schemas
│   ├── discovery/
│   │   ├── detectors/   # Detection contracts
│   │   └── profile/     # ClusterProfile types
│   └── modeling/        # Prediction/simulation (future)
├── deploy/              # Helm/manifests (future)
├── docs/                # Documentation
└── test/
    └── e2e/
        ├── kind/        # Cluster setup scripts
        ├── apps/        # Test microservices
        └── scenarios/   # E2E test scenarios
```

## Common Tasks

```bash
# Format code
task fmt

# Run tests
task test

# Build binaries
task build

# Run linter
task lint

# Run vet
task vet

# Generate JSON schemas
task generate

# Run all CI checks
task ci
```

## Adding a New Telemetry Adapter

To add support for a new telemetry source (e.g., Istio):

### 1. Create the Adapter Package

```
pkg/adapters/istio/
├── discovery.go      # Implement adapters.DiscoveryAdapter
└── discovery_test.go
```

### 2. Implement the Discovery Interface

```go
package istio

import (
    "context"
    "github.com/erickivel/oksg/pkg/adapters"
    "github.com/erickivel/oksg/pkg/discovery/profile"
)

type DiscoveryAdapter struct {
    // client for Istio API
}

var _ adapters.DiscoveryAdapter = (*DiscoveryAdapter)(nil)

func (d *DiscoveryAdapter) Name() string {
    return "istio-discovery-adapter"
}

func (d *DiscoveryAdapter) Discover(ctx context.Context) (*profile.ClusterProfile, error) {
    // Query Istio resources
    // Detect telemetry capabilities
    // Return enriched ClusterProfile
}
```

### 3. Extend the Source Planner

Update `pkg/adapters/otel/source_planner.go` to handle new source types:

```go
func (p *planner) Plan(ctx context.Context, clusterProfile *profile.ClusterProfile) (*Plan, error) {
    // ... existing code ...
    
    // Add Istio receiver if detected
    if hasIstioSource(clusterProfile.Sources) {
        agentReceivers["istio"] = map[string]any{
            // Istio-specific config
        }
        pipelineReceivers = append(pipelineReceivers, "istio")
    }
}
```

### 4. Add Tests

Write unit tests with mocked dependencies and add an e2e scenario if applicable.

## Extending the ClusterProfile

To add new fields to the discovery profile:

### 1. Update the Profile Types

Edit `pkg/discovery/profile/profile.go`:

```go
type ClusterProfile struct {
    // ... existing fields ...
    NewField []NewType `json:"new_field,omitempty"`
}

type NewType struct {
    // Define structure
}
```

### 2. Update the Discovery Adapter

Populate the new fields in `pkg/adapters/kubernetes/discovery.go`.

### 3. Update Tests

Add test coverage in `pkg/discovery/profile/profile_test.go` and `pkg/adapters/kubernetes/discovery_test.go`.

## Adding OTel Collector Templates

Templates live in `pkg/adapters/otel/templates/`:

1. Create a new `.yaml.tmpl` file
2. Update `templates.go` to embed the new file
3. Update `pkg/collector/configgen/generator.go` to render it

## Code Style Guidelines

- Follow standard Go conventions
- Use `gofmt` / `goimports` for formatting
- Keep functions small and focused
- Write table-driven tests
- Avoid adding dependencies without concrete need
- Document exported types and functions

## Git Workflow

1. Create a feature branch from `development`
2. Make changes with focused commits
3. Run `task ci` before pushing
4. Create a PR targeting `development`
5. Merge to `main` for releases

## Debugging Tips

### Inspect Discovery Output

```bash
./discovery-job --mode=discover --kubeconfig=$HOME/.kube/config | jq .
```

### View Generated Manifests

```bash
./discovery-job --mode=plan --profile=profile.json | kubectl apply --dry-run=client -f -
```

### Check OTel Collector Logs

```bash
kubectl logs -n oksg-system -l app=otel-collector-agent -f
kubectl logs -n oksg-system -l app=otel-collector-gateway -f
```

### Port-Forward Gateway

```bash
kubectl port-forward -n oksg-system svc/otel-collector-gateway 4317:4317
```
