# OKSG Documentation

This folder documents the currently implemented OKSG slice: Kubernetes discovery, source planning, Helm values generation, Helm-managed deployment of the OpenTelemetry collection plane, and the current runtime telemetry flow through agent, gateway, and backend ingest.

## Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | Current layers, implemented boundaries, Helm deployment model, logging policy |
| [Discovery Flow](discovery-flow.md) | Discovery to release-values flow and Helm render/install sequence |
| [Runtime Flow](runtime-flow.md) | Post-deploy telemetry flow through agent, gateway, backend ingest, and raw-envelope routing |
| [Testing](testing.md) | Unit tests, kind-based e2e tasks, and scenario validation |
| [Development](development.md) | Project structure, common workflows, chart layout, and maintenance rules |

## Quick Links

- [README](../README.md)
- [AGENTS.md](../AGENTS.md)
- [GUIDELINE.md](../GUIDELINE.md)
- [test/e2e/apps](../test/e2e/apps/)
- [test/e2e/scenarios](../test/e2e/scenarios/)

## Current Status

- The current slice includes discovery-driven collector planning plus the first deployed collection-plane runtime.
- `cmd/discovery-job` orchestrates discovery, planning, and deploy operations.
- `cmd/backend` is now a real OTLP ingest runtime.
- `cmd/controller` remains a placeholder.
- OTel resources are packaged through the Helm chart in `deploy/helm/oksg-otel`.

## Flow At A Glance

```text
Kubernetes Cluster
       |
       v
Discovery Adapter -> ClusterProfile -> Source Planner -> Helm Values -> Helm Release -> Runtime Flow
```

## Documentation Rule

Keep these docs aligned with the real implementation. When code changes make text, commands, diagrams, or examples stale, update or remove the stale docs in the same change.
