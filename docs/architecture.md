# Architecture Overview

OKSG (Observability-Driven Kubernetes System Graph) transforms heterogeneous runtime observability signals into a canonical, time-aware system graph that combines structure and behavior.

## System Layers

The platform is organized into six architectural layers:

```
┌─────────────────────────────────────────────────────────────────┐
│                    6. Control and API Layer                     │
│              (Query APIs, Graph Exploration, Reports)           │
├─────────────────────────────────────────────────────────────────┤
│                 5. Modeling and Prediction Layer                │
│           (Scenarios, Predictors, Simulation) [Future]          │
├─────────────────────────────────────────────────────────────────┤
│                    4. Persistence Layer                         │
│          (Metrics Store, Topology Store, Metadata Store)        │
├─────────────────────────────────────────────────────────────────┤
│                3. Semantic Processing Layer                     │
│        (Normalization, Canonicalization, Graph Assembly)        │
├─────────────────────────────────────────────────────────────────┤
│                    2. Collection Plane                          │
│         (OTel Agents, Gateway, Pipelines, Exporters)            │
├─────────────────────────────────────────────────────────────────┤
│               1. Cluster Integration Layer                      │
│    (Kubernetes, Prometheus, Istio, OTLP, OBI Adapters)          │
└─────────────────────────────────────────────────────────────────┘
```

## Current Implementation (Phase 2)

Phase 2 focuses on **Discovery-Driven OTel Collection**, implementing:

- **Kubernetes Discovery Adapter**: Queries cluster API for nodes, namespaces, deployments, and services
- **Prometheus Endpoint Detection**: Identifies services with `prometheus.io/scrape` annotations
- **OTel Source Planner**: Generates collector configurations based on discovered telemetry sources
- **Config Generator**: Renders Kubernetes manifests from OTel plans
- **Deployer**: Applies generated manifests to the cluster

## Data Flow

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Kubernetes  │────▶│   Discovery  │────▶│    Source    │────▶│    Config    │
│   Cluster    │     │   Adapter    │     │   Planner    │     │  Generator   │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
                            │                    │                    │
                            ▼                    ▼                    ▼
                     ClusterProfile         OTel Plan           K8s Manifests
                                                                     │
                                                                     ▼
                                                              ┌──────────────┐
                                                              │   Deployer   │
                                                              │  (kubectl)   │
                                                              └──────────────┘
```

## Key Design Principles

1. **Minimal Dependencies**: Add packages only when concretely needed
2. **Explicit Contracts**: Define interfaces before implementations
3. **Discovery-Driven**: Let cluster state determine collector configuration
4. **Modular Adapters**: Keep technology-specific code isolated
5. **DRY Core Model**: Define canonical entities, metrics, and relations once
6. **Testability First**: Small interfaces, deterministic IDs, validation helpers
7. **Deferred Prediction**: Treat prediction as a future consumer, not an early shortcut

## Component Responsibilities

| Component | Package | Responsibility |
|-----------|---------|----------------|
| Discovery Adapter | `pkg/adapters/kubernetes` | Query K8s API, detect Prometheus endpoints |
| Source Planner | `pkg/adapters/otel` | Generate OTel Collector configs from profile |
| Config Generator | `pkg/collector/configgen` | Render K8s manifests from OTel plans |
| Deployer | `pkg/collector/deployer` | Apply manifests via kubectl |
| Discovery Job | `cmd/discovery-job` | CLI orchestrating the pipeline |

## OTel Collector Architecture

The generated OTel Collector setup consists of:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                     oksg-system namespace                    ││
│  │                                                              ││
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       ││
│  │  │ Agent (Node1)│  │ Agent (Node2)│  │ Agent (NodeN)│       ││
│  │  │  DaemonSet   │  │  DaemonSet   │  │  DaemonSet   │       ││
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       ││
│  │         │                 │                 │                ││
│  │         └─────────────────┼─────────────────┘                ││
│  │                           │ OTLP                             ││
│  │                           ▼                                  ││
│  │                    ┌──────────────┐                          ││
│  │                    │   Gateway    │                          ││
│  │                    │  Deployment  │                          ││
│  │                    └──────────────┘                          ││
│  └─────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
```

- **Agents (DaemonSet)**: Run on each node, collect kubeletstats and scrape Prometheus endpoints
- **Gateway (Deployment)**: Receives OTLP from agents, processes and exports metrics
