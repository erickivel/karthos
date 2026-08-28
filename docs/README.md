# OKSG Documentation

Implementation documentation for OKSG. Treat these files as the source of
truth: when code changes invalidate text, commands, diagrams, or examples,
update them in the same change.

## Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | Layers, component boundaries, Helm deployment model, logging policy |
| [Discovery Flow](discovery-flow.md) | Discovery → release-values → Helm install sequence |
| [Runtime Flow](runtime-flow.md) | Post-deploy telemetry path through agent, gateway, backend ingest |
| [Metrics Pipeline](metrics-pipeline.md) | Catalog, source-module parsing, resolver |
| [API Reference](api-reference.md) | REST API endpoints, query params, response shapes |
| [Frontend Design Spec](frontend-design-spec.md) | Frontend stack, design system, projection modes |
| [Testing](testing.md) | Unit tests, kind-based e2e tasks, scenario validation |
| [Development](development.md) | Project structure, common commands, frontend dev loop |

## Quick Links

- [README](../README.md)
- [AGENTS.md](../AGENTS.md)
- [GUIDELINE.md](../GUIDELINE.md)
- [test/e2e/apps](../test/e2e/apps/)
- [test/e2e/scenarios](../test/e2e/scenarios/)

## Flow At A Glance

```text
Kubernetes Cluster
      |
      v
Discovery Adapter -> ClusterProfile -> Source Plan -> Generic Collection Plan -> OTel Adapter -> Generated Helm Values
                                                              |
                                                              v
                                              Helm source-contract ConfigMap -> Helm Release
                                                                 |
                                                                 v
                       Backend (strict validation -> OTLP ingest -> Normalizer -> Topology builders -> Stores -> REST API)
                                          |
                                          v
                              React frontend (/, /topology, /metrics)
```
