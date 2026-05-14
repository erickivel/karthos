# Metrics Explorer Applicability

This folder owns the pure rules that decide **which tabs and which cards the
Metrics Explorer should render** for a given selection.

The backend metric catalog is the source of truth. `CanonicalMetric.scopes`
declares whether a metric supports entity queries, edge queries, or both.
`CanonicalMetric.applies_to` declares which graph entity kinds an entity-scoped
metric is valid for.

For `workload` entities, the graph label `k8s.workload.type` further narrows
controller-specific metric families. A Deployment workload renders
`k8s.deployment.*`, a Job renders `k8s.job.*`, and so on. Missing subtype
metadata fails open so older graph snapshots keep the previous generic workload
behavior.

## The Three Card States

| State | Meaning | Driven by |
| --- | --- | --- |
| **Hidden** | Metric is structurally inapplicable to this kind. Card never renders. | `scopes` + `applies_to` from `/api/v1/metrics/catalog` |
| **Shown with data** | Metric is applicable and the API returned at least one sample. | The query result |
| **Shown empty** | Metric is applicable but the API returned zero samples in the window. | Card-level empty state in `MetricCard` |

Category tabs are derived from the applicable catalog metrics for the current
selection. If a selected container has resource and health metrics, only those
tabs are shown. If a selected ConfigMap has no applicable metrics, no category
tabs are shown.

## Files

- `applicability.ts` — pure functions consumed by the Explorer:
  - `visibleCategoriesForSelection(metrics, kind, mode, context)`
  - `isMetricApplicableToKind(metric, kind, context)`
  - `applicableMetrics(metrics, category, kind, mode, context)`
- `applicability.test.ts` — unit tests covering the contract.

## How to Extend

### Add a New Entity Kind

1. Add the kind to `EntityKind` in `web/src/types/graph.ts`.
2. Add the backend entity kind in `pkg/core/entities/entity.go`.
3. Add the kind to each relevant catalog metric's `applies_to` mapping in
   `pkg/core/metrics/catalog.go`.

### Add a New Metric

1. Add the metric to the backend catalog with its `scopes`.
2. Add `applies_to` metadata for every entity kind the metric can be queried
   against. Edge-only metrics intentionally omit `applies_to`.
3. Add or update frontend applicability tests for the expected entity kind.
4. If the metric applies to `workload`, add or verify the `k8s.workload.type`
   subtype rule when the metric is controller-specific.

### Add a New Metric Category

1. Update `MetricCategory` in `web/src/types/metrics.ts`.
2. Update the `ALL_CATEGORIES` constant in `applicability.ts`.
3. Add catalog metrics using the new category; tabs will appear automatically
   when those metrics apply to the current selection.

## Drift Detection

Current backend tests require every entity-scoped catalog metric to declare at
least one `applies_to` kind. The frontend keeps a compatibility fail-open path
for older catalog responses and logs a dev-only warning when that metadata is
missing.
