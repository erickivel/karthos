# Development Guide

This guide focuses on the current implementation shape rather than future-only layouts.

## Prerequisites

- Go 1.23+
- [Task](https://taskfile.dev/)
- Docker
- kind
- kubectl
- helm

## Current Structure

```text
cmd/
  discovery-job/   implemented discovery/plan/deploy CLI
  backend/         placeholder
  controller/      placeholder
  schema-gen/      schema helper

pkg/
  adapters/kubernetes/   discovery adapter
  adapters/otel/         source planner
  collector/configgen/   Helm values builder
  collector/deployer/    Helm runner
  core/logging/          shared JSON stdout logger
  discovery/profile/     ClusterProfile types

deploy/helm/oksg-otel/
  Chart.yaml
  values.yaml
  values.schema.json
  templates/
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
./bin/discovery-job --mode=plan --profile=profile.json --values-output=otel-values.yaml --rendered-output=./bin/e2e/rendered
./bin/discovery-job --mode=deploy --profile=profile.json --release-name=oksg-otel --chart-path=./deploy/helm/oksg-otel --namespace=oksg-system
```

Notes:

- use file outputs for `discover` and `plan` when you need clean machine-readable artifacts
- `plan` now emits Helm values, not raw manifests
- `deploy` uses Helm, not `kubectl apply`

## Helm Ownership

The chart owns Kubernetes resource rendering. When changing deployment behavior:

1. update planner output only when collection behavior changes
2. update values builder when deployment intent changes
3. update chart templates when Kubernetes resource shape changes
4. update `values.schema.json` whenever values shape changes

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
```
