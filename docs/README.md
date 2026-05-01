# OKSG Documentation

This folder documents the currently implemented OKSG slice: Kubernetes discovery, source planning, Helm values generation, Helm-managed deployment of the OpenTelemetry collection plane, and the current runtime telemetry flow through agent, gateway, and backend ingest.

## Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | Current layers, implemented boundaries, Helm deployment model, logging policy |
| [Discovery Flow](discovery-flow.md) | Discovery to release-values flow and Helm render/install sequence |
| [Runtime Flow](runtime-flow.md) | Post-deploy telemetry flow through agent, gateway, backend ingest, and raw-envelope routing |
| [Metrics Pipeline](metrics-pipeline.md) | Catalog, dialect translation, source registry, and resolver-based source arbitration |
| [API Reference](api-reference.md) | REST API endpoints, query params, and response shapes |
| [Frontend Design Spec](frontend-design-spec.md) | Frontend stack, design system, projection modes, and layout strategies |
| [Testing](testing.md) | Unit tests, kind-based e2e tasks, and scenario validation |
| [Development](development.md) | Project structure, common workflows, chart layout, and maintenance rules |

## Quick Links

- [README](../README.md)
- [AGENTS.md](../AGENTS.md)
- [GUIDELINE.md](../GUIDELINE.md)
- [test/e2e/apps](../test/e2e/apps/)
- [test/e2e/scenarios](../test/e2e/scenarios/)

## Current Status

- `cmd/discovery-job` orchestrates discover, plan, and deploy operations.
- `cmd/backend` runs the OTLP ingest, semantic normalization pipeline, REST API, and a periodic discovery-refresh loop that reseeds structural topology before TTL aging removes it.
- `cmd/controller` remains a placeholder.
- `web/` is the React frontend, packaged in the same Helm chart and served behind nginx with an `/api/*` proxy.
- OTel collector resources, the backend, the frontend, and optional storage subcharts (PostgreSQL, VictoriaMetrics) are all packaged through the Helm chart in `deploy/helm/oksg-otel`.
- The metrics pipeline (`pkg/core/metrics`) does catalog → dialect translation → source-resolver arbitration end-to-end; the resolver output drives both planner receiver/connector deployment and runtime sample-mismatch drops in the normalizer.

## Flow At A Glance

```text
Kubernetes Cluster
       |
       v
Discovery Adapter -> ClusterProfile -> [Resolver] -> Source Planner -> Helm Values -> Helm Release
                                          |
                                          v
                               Backend (OTLP ingest -> Normalizer -> Topology builders -> Stores -> REST API)
                                          |
                                          v
                                    React frontend (/, /topology, /metrics)
```

## Documentation Rule

Keep these docs aligned with the real implementation. When code changes make text, commands, diagrams, or examples stale, update or remove the stale docs in the same change.
