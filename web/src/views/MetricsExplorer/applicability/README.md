# Metrics Explorer applicability

This folder owns the rules that decide **which tabs and which cards the
Metrics Explorer should render** for a given selection.

It exists so that the Explorer never shows a tab or a card that is
*structurally impossible* for the selected entity (e.g. CPU on a ConfigMap,
HTTP traffic on a PVC) and so that genuinely empty cards render an
informative empty state instead of a blank chart.

## The three card states

| State                  | Meaning                                                                 | Driven by                                |
| ---------------------- | ----------------------------------------------------------------------- | ---------------------------------------- |
| **Hidden**             | Metric is structurally inapplicable to this kind. Card never renders.   | `KIND_CATEGORIES` + `SOURCE_KINDS`       |
| **Shown with data**    | Metric is applicable and the API returned at least one sample.          | The query result.                        |
| **Shown empty**        | Metric is applicable but the API returned zero samples in the window.   | Card-level empty state in `MetricCard`.  |

A category tab is hidden if `KIND_CATEGORIES[selectedKind]` does not
include it. Within a visible tab, an individual card is hidden if none of
its `metric.sources[].id` map to the selected kind via `SOURCE_KINDS`.

## Files

- `kindCategories.ts` — coarse `EntityKind → MetricCategory[]`. Drives tab
  visibility. Edit when an entity kind gains or loses an entire metric
  category.
- `sourceKinds.ts` — fine-grained `SourceId → EntityKind[] | '*'`. Drives
  card visibility. Edit when a new metric source is introduced or an
  existing source starts emitting for a new kind.
- `applicability.ts` — pure functions consumed by the Explorer:
  - `visibleCategoriesForKind(kind, mode)`
  - `isMetricApplicableToKind(metric, kind)`
  - `applicableMetrics(metrics, category, kind, mode)`
- `applicability.test.ts` — unit tests covering the contract.

## How to extend

### Add a new entity kind

1. Add the kind to `EntityKind` in `web/src/types/graph.ts` (if not already
   present — the type is the source of truth).
2. Add a row to `KIND_CATEGORIES` listing the categories that make sense
   for this kind. An empty array is valid (e.g. `secret: []`).
3. If any source in `SOURCE_KINDS` should also emit for this kind, append
   the kind to that source's value.

### Add a new metric source

1. Pick the source id used in `pkg/core/metrics/canonical.go` and surfaced
   on `CanonicalMetric.sources[].id` (e.g. `"prometheus.app"`).
2. Add a row to `SOURCE_KINDS` listing the kinds that source emits for.
   Use `'*'` only as a deliberate escape hatch for catch-all exporters.

### Add a new metric category

1. Update `MetricCategory` in `web/src/types/metrics.ts`.
2. Add the new category to the `KIND_CATEGORIES` rows where it should
   appear.
3. Update the `ALL_CATEGORIES` constant in `applicability.ts`.

## The `'*'` wildcard

A wildcard in `SOURCE_KINDS` means *"this source can emit for any entity
kind, do not gate cards by kind"*. Use it sparingly — it disables the
fine-grained filter for every metric whose sources include it. Reasonable
uses: generic Prometheus jobs, a future "user-supplied" source. Avoid for
sources whose targets are statically knowable.

## Drift detection

Adding a metric to the backend catalog without listing its source id in
`SOURCE_KINDS` is a recurring failure mode. Two safeguards:

1. **Fail-open in production.** Unknown source ids are treated as
   applicable to every kind, so the worst case is a card showing where it
   shouldn't, never the reverse.
2. **`console.warn` in dev mode.** The first time `isMetricApplicableToKind`
   sees an unknown source id during a `vite dev` session it logs a single
   warning naming the source id and pointing at this folder. Watch the
   browser console after a backend catalog change.

## Future migration path

When the backend source registry exposes a `produces_kinds` field per
source (server-of-truth), replace the static `SOURCE_KINDS` map with an
API fetch (e.g. `/api/v1/metrics/sources`) and a small hook. The function
signatures in `applicability.ts` stay identical, so consumers do not
change.
