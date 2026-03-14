# OKSG Documentation

This folder documents the currently implemented OKSG slice: Kubernetes discovery, source planning, Helm values generation, and Helm-managed deployment of OpenTelemetry collector agent/gateway resources.

## Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | Current layers, implemented boundaries, Helm deployment model, logging policy |
| [Discovery Flow](discovery-flow.md) | Discovery to release-values flow and Helm render/install sequence |
| [Testing](testing.md) | Unit tests, kind-based e2e tasks, and scenario validation |
| [Development](development.md) | Project structure, common workflows, chart layout, and maintenance rules |

## Quick Links

- [README](../README.md)
- [AGENTS.md](../AGENTS.md)
- [test/e2e/apps](../test/e2e/apps/)
- [test/e2e/scenarios](../test/e2e/scenarios/)

## Current Status

- Phase 2 currently implements discovery-driven collector planning.
- The only implemented OKSG runtime binary today is `cmd/discovery-job`.
- `cmd/backend` and `cmd/controller` remain placeholders.
- OTel resources are now packaged through the Helm chart in `deploy/helm/oksg-otel`.

## Flow At A Glance

```text
Kubernetes Cluster
       |
       v
Discovery Adapter -> ClusterProfile -> Source Planner -> Helm Values -> Helm Release
```

## Documentation Rule

Keep these docs aligned with the real implementation. When code changes make text, commands, diagrams, or examples stale, update or remove the stale docs in the same change.
