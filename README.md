# OKSG — Observability-Driven Kubernetes System Graph

OKSG is the foundation of a Kubernetes-native observability-to-graph platform that reconstructs a live, explorable system graph from runtime telemetry and cluster discovery data.

## What It Does

The project is designed to:

- discover services, workloads, pods, nodes, and related cluster resources
- ingest runtime metrics and interaction signals
- normalize heterogeneous observability data into stable internal contracts
- reconstruct a time-aware topology graph with metrics attached to nodes and edges
- preserve a modular boundary for later prediction, simulation, and reporting features

Phase 1 focuses on the architectural kernel: schemas, interfaces, contracts, and repository structure.

## Architecture Overview

The long-term architecture is organized into six layers:

1. Cluster Integration Layer
2. Collection Plane
3. Semantic Processing Layer
4. Persistence Layer
5. Modeling and Prediction Layer
6. Control and API Layer

This repository currently implements the Phase 1 foundation for those layers:

- canonical entities
- canonical metrics
- raw telemetry envelopes
- topology and edge contracts
- discovery profile contracts
- adapter, collector, persistence, query, and predictor interfaces

## Project Status

Current phase: `Phase 1 — Foundation and Contracts`

Planned roadmap:

1. Foundation and Contracts
2. Cluster Integration and Discovery
3. Collection Plane Implementation
4. Semantic Processing and Canonicalization
5. Topology and Graph Reconstruction
6. Visualizer and Query APIs
7. Post-TCC evolution toward prediction and simulation

## Prerequisites

- Go `1.23+`
- [Task](https://taskfile.dev/)

Optional tooling:

- `golangci-lint`

## Getting Started

```bash
go mod tidy
task test
task build
task generate
```

## Repository Layout

```text
cmd/
  backend/         Backend API/service entrypoint
  controller/      Future controller/operator entrypoint
  discovery-job/   Future discovery job entrypoint
  schema-gen/      JSON Schema generation helper

pkg/
  adapters/        Technology-specific integration boundaries
  backend/         Ingest, query, and persistence contracts
  collector/       Collection-plane contracts and placeholders
  core/            Canonical entities, metrics, topology, schema, and registry
  discovery/       Detection and cluster profile contracts
  modeling/        Deferred prediction and simulation contracts

deploy/
  helm/            Helm packaging placeholder
  manifests/       Raw manifest placeholder
  examples/        Example deployment placeholder
```

## License

License is intentionally left undecided at this stage and should be finalized explicitly later.
