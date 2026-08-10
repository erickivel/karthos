# Discovery and source planning

Discovery produces a capability-only snapshot. It does not serialize cluster
topology or collector recommendations. Live Kubernetes topology is owned by
the `kubernetes.structure` module at backend runtime.

```mermaid
sequenceDiagram
    participant CLI as discovery-job
    participant K as Kubernetes API
    participant M as compiled source modules
    participant P as capability profile
    participant R as generic resolver
    participant SP as resolved source plan
    participant H as Helm compiler

    CLI->>M: discover
    M->>K: direct source-specific probes
    M-->>P: all seven capability records
    CLI->>P: write profile.json
    CLI->>R: plan(profile)
    R-->>SP: metric assignments and selected modules
    SP->>H: selected contributions
    H-->>CLI: otel-values.yaml + source-plan sidecar
```

## Capability profile

The profile contains cluster identity, discovery time, and exactly one record
for every compiled module:

- `kubernetes.structure` (non-metric)
- `kubelet`, `host-os`, `kube-cluster`
- `prometheus.app`, `mesh.istio`, `traces.servicegraph`

Each record has a stable source ID, `direct` or `derived` kind, availability
status, signal classes, and bounded evidence. The profile records unavailable
modules too, so it is an auditable result of successful detection rather than
a list of only positive findings.

`traces.servicegraph` is derived: its evidence identifies a discovered
trace-capable OTLP producer, while the selected gateway connector produces the
metric source.

Detector API failures fail discovery. They are never interpreted as an absent
source. Mesh detection is `partial` when its injection/proxy requirements are
not complete and is therefore not eligible for metric arbitration.

## Source planning

The metric catalog declares eligible sources and priority. The generic resolver
uses only available capability records to select at most one source for each
canonical metric. There are no operator source overrides.

The resulting sidecar is written beside `--values-output` as
`<values-output>.source-plan.json` unless `--resolved-plan-output` is given.
It includes:

- a fingerprint of the normalized capability profile;
- selected source module IDs;
- the complete canonical metric-to-source assignment.

The compiler consumes the same result to include only selected receiver,
connector, scrape, pipeline, and source-stamp fragments. Helm receives the
generated collector configuration; the E2E deployment task mounts the profile
and plan together as the `oksg-source-contract` ConfigMap.

## Runtime contract

Backend startup requires both mounted files. It recomputes the profile
fingerprint, rejects a mismatch, and validates selected and assigned source
IDs against the compiled source inventory. It uses the mounted assignment for
ingestion policy and does not resolve sources again.

The backend independently creates an in-cluster Kubernetes client for
`kubernetes.structure`. That module refreshes the canonical structural graph
at startup and on `OKSG_STRUCTURE_REFRESH_INTERVAL`; the interval must be
positive and strictly shorter than `OKSG_NODE_TTL` whenever TTL is enabled.
