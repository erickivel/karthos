# OKSG Documentation

Welcome to the OKSG (Observability-Driven Kubernetes System Graph) documentation.

## Contents

| Document | Description |
|----------|-------------|
| [Architecture](architecture.md) | System layers, component responsibilities, and design principles |
| [Discovery Flow](discovery-flow.md) | Sequence diagram and detailed pipeline documentation |
| [Testing](testing.md) | Unit tests, e2e testing with kind, and test scenarios |
| [Development](development.md) | Setup, project structure, and contribution guidelines |

## Quick Links

- **Getting Started**: See the main [README](../README.md)
- **Agent Guidelines**: See [AGENTS.md](../AGENTS.md)
- **E2E Test Apps**: See [test/e2e/apps/](../test/e2e/apps/)
- **Test Scenarios**: See [test/e2e/scenarios/](../test/e2e/scenarios/)

## Project Status

- **Phase 1** (Completed): Foundation and contracts
- **Phase 2** (Current): Discovery-driven OTel collection

## Architecture at a Glance

```
Kubernetes Cluster
       │
       ▼
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Discovery   │────▶│    Source    │────▶│    Config    │────▶ K8s Manifests
│   Adapter    │     │   Planner    │     │  Generator   │
└──────────────┘     └──────────────┘     └──────────────┘
       │
       ▼
 ClusterProfile
 (nodes, services,
  prometheus targets,
  recommended receivers)
```

The system discovers cluster resources, detects telemetry capabilities (currently Prometheus endpoints), and dynamically generates OpenTelemetry Collector configurations.
