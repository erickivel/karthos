package api

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

// catalogStatsLookback is the fixed window used by the /api/v1/metrics/catalog/stats
// endpoint. One hour is short enough that "no data" rows reliably mean a metric
// is not flowing, and long enough that low-rate signals still register.
const catalogStatsLookback = time.Hour

// MetricsHandler serves metric query and catalog endpoints.
type MetricsHandler struct {
	metricStore persistence.MetricStore
	catalog     map[string]metrics.CanonicalMetric
	logger      *logging.Logger
}

// newMetricsHandler creates a MetricsHandler backed by the canonical catalog.
func newMetricsHandler(store persistence.MetricStore, logger *logging.Logger) *MetricsHandler {
	return &MetricsHandler{
		metricStore: store,
		catalog:     metrics.CatalogByName(),
		logger:      logger,
	}
}

// RegisterRoutes registers metrics endpoints on the given mux.
func (h *MetricsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/metrics", h.handleQueryMetrics)
	mux.HandleFunc("GET /api/v1/metrics/catalog", h.handleGetCatalog)
	mux.HandleFunc("GET /api/v1/metrics/catalog/stats", h.handleGetCatalogStats)
	mux.HandleFunc("GET /api/v1/metrics/series", h.handleGetMetricSeries)
}

type enrichedSample struct {
	MetricName string            `json:"metric_name"`
	EntityID   entities.EntityID `json:"entity_id"`
	Value      float64           `json:"value"`
	// ValueLabel is the public string translation of an integer-encoded enum
	// value (see pkg/core/metrics/health_status.go). Set only on samples for
	// metrics where IsHealthStatusMetric reports true; omitted otherwise.
	ValueLabel string            `json:"value_label,omitempty"`
	Timestamp  string            `json:"timestamp"`
	Labels     map[string]string `json:"labels,omitempty"`
	Unit       metrics.Unit      `json:"unit,omitempty"`
	Edge       *edgeRef          `json:"edge,omitempty"`
}

type metricSeries struct {
	MetricName  string           `json:"metric_name"`
	Unit        metrics.Unit     `json:"unit,omitempty"`
	Category    string           `json:"category,omitempty"`
	Description string           `json:"description,omitempty"`
	Samples     []enrichedSample `json:"samples"`
	Edge        *edgeRef         `json:"edge,omitempty"`
}

type edgeRef struct {
	SourceID entities.EntityID `json:"source_id"`
	TargetID entities.EntityID `json:"target_id"`
}

type effectiveMetricQuery struct {
	Aggregation metrics.AggregationMode `json:"aggregation"`
	Window      string                  `json:"window,omitempty"`
	Unit        metrics.Unit            `json:"-"`
}

// queryEnrichedSamples parses the common metrics query parameters, issues the
// store query, and returns enriched+sorted (descending) samples. On HTTP-level
// failure the handler has already written the error response and the second
// return is false.
func (h *MetricsHandler) queryEnrichedSamples(w http.ResponseWriter, r *http.Request) ([]enrichedSample, effectiveMetricQuery, bool) {
	if h.metricStore == nil {
		writeError(w, http.StatusNotFound, "metric store not configured")
		return nil, effectiveMetricQuery{}, false
	}

	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' parameter: expected RFC3339 format")
		return nil, effectiveMetricQuery{}, false
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' parameter: expected RFC3339 format")
		return nil, effectiveMetricQuery{}, false
	}

	metricName := r.URL.Query().Get("metric_name")
	if metricName == "" {
		writeError(w, http.StatusBadRequest, "metric_name query parameter is required")
		return nil, effectiveMetricQuery{}, false
	}
	metric, exists := h.catalog[metricName]
	if !exists {
		writeError(w, http.StatusBadRequest, "unsupported metric_name")
		return nil, effectiveMetricQuery{}, false
	}
	resolved, err := metrics.ResolveAggregation(metric, r.URL.Query().Get("aggregation"), r.URL.Query().Get("window"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, effectiveMetricQuery{}, false
	}
	effective := effectiveMetricQuery{Aggregation: resolved.Mode, Unit: resolved.Unit}
	if resolved.Window > 0 {
		effective.Window = metrics.FormatAggregationWindow(resolved.Window)
	}

	var entityIDs []entities.EntityID
	for _, id := range r.URL.Query()["entity_id"] {
		entityID := entities.EntityID(id)
		if !entities.IsCanonicalID(entityID) {
			writeError(w, http.StatusBadRequest, "entity_id must be a canonical entity ID")
			return nil, effectiveMetricQuery{}, false
		}
		entityIDs = append(entityIDs, entityID)
	}
	edgeSourceID := entities.EntityID(r.URL.Query().Get("edge_source_id"))
	if edgeSourceID != "" && !entities.IsCanonicalID(edgeSourceID) {
		writeError(w, http.StatusBadRequest, "edge_source_id must be a canonical entity ID")
		return nil, effectiveMetricQuery{}, false
	}
	edgeTargetID := entities.EntityID(r.URL.Query().Get("edge_target_id"))
	if edgeTargetID != "" && !entities.IsCanonicalID(edgeTargetID) {
		writeError(w, http.StatusBadRequest, "edge_target_id must be a canonical entity ID")
		return nil, effectiveMetricQuery{}, false
	}

	q := query.MetricQuery{
		EntityIDs:    entityIDs,
		MetricName:   metricName,
		From:         from,
		To:           to,
		EdgeSourceID: edgeSourceID,
		EdgeTargetID: edgeTargetID,
		Aggregation:  resolved.Mode,
		Window:       resolved.Window,
		Unit:         resolved.Unit,
	}

	samples, err := h.metricStore.Query(r.Context(), q)
	if err != nil {
		h.logger.Warn("failed to query metrics", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query metrics")
		return nil, effectiveMetricQuery{}, false
	}

	enriched := make([]enrichedSample, 0, len(samples))
	for _, s := range samples {
		unit := resolved.Unit
		// A generic resolved unit marks a heterogeneous family whose physical
		// unit is carried by the sample (currently ResourceQuota). Fixed-unit
		// metrics, especially counter modes, must keep the mode-aware resolved
		// unit even when raw storage carries a presentation unit that differs.
		if resolved.Unit == metrics.UnitResource && metrics.IsValidUnit(s.Unit) {
			unit = s.Unit
		}
		enriched = append(enriched, enrichedSample{
			MetricName: s.MetricName,
			EntityID:   s.EntityID,
			Value:      s.Value,
			ValueLabel: s.ValueLabel,
			Timestamp:  s.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			Labels:     s.Labels,
			Unit:       unit,
			Edge:       edgeRefFromLabels(s.Labels),
		})
	}

	slices.SortFunc(enriched, func(a, b enrichedSample) int {
		return cmp.Compare(b.Timestamp, a.Timestamp) // descending
	})
	return enriched, effective, true
}

func (h *MetricsHandler) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	enriched, effective, ok := h.queryEnrichedSamples(w, r)
	if !ok {
		return
	}

	total := len(enriched)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(enriched, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
		"query":   effective,
		"samples": page,
		"total":   total,
	}, nextToken)
}

func (h *MetricsHandler) handleGetCatalog(w http.ResponseWriter, r *http.Request) {
	catalog := metrics.DefaultCatalog()
	categoryFilter := metrics.MetricCategory(r.URL.Query().Get("category"))

	if categoryFilter != "" {
		var filtered []metrics.CanonicalMetric
		for _, m := range catalog {
			if m.Category == categoryFilter {
				filtered = append(filtered, m)
			}
		}
		catalog = filtered
	}
	if catalog == nil {
		catalog = []metrics.CanonicalMetric{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": catalog,
		"total":   len(catalog),
	})
}

// catalogStatsResponse is the JSON shape returned by GET /api/v1/metrics/catalog/stats.
// Always contains one entry per canonical metric (zero-filled across declared
// sources) so the frontend renders coverage badges without a second request.
type catalogStatsResponse struct {
	LookbackSeconds int                 `json:"lookback_seconds"`
	Stats           []catalogStatsEntry `json:"stats"`
}

type catalogStatsEntry struct {
	MetricName string                 `json:"metric_name"`
	BySource   []catalogStatsBySource `json:"by_source"`
}

type catalogStatsBySource struct {
	SourceID    string `json:"source_id"`
	Priority    int    `json:"priority,omitempty"`
	SampleCount int64  `json:"sample_count"`
	// LastSeenAt is RFC3339 if any sample was observed in the window, otherwise nil.
	LastSeenAt *string `json:"last_seen_at"`
}

func (h *MetricsHandler) handleGetCatalogStats(w http.ResponseWriter, r *http.Request) {
	if h.metricStore == nil {
		writeError(w, http.StatusNotFound, "metric store not configured")
		return
	}

	rawStats, err := h.metricStore.QueryCatalogStats(r.Context(), catalogStatsLookback)
	if err != nil {
		h.logger.Warn("failed to query catalog stats", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query catalog stats")
		return
	}

	// Index store output by (metric, source) for O(1) lookup during zero-fill.
	type srcKey struct{ metric, source string }
	indexed := map[srcKey]persistence.SourceStat{}
	for _, cs := range rawStats {
		for _, ss := range cs.BySource {
			indexed[srcKey{metric: cs.MetricName, source: ss.SourceID}] = ss
		}
	}

	formatLastSeen := func(t time.Time) *string {
		if t.IsZero() {
			return nil
		}
		s := t.UTC().Format(time.RFC3339)
		return &s
	}

	catalog := metrics.DefaultCatalog()
	entries := make([]catalogStatsEntry, 0, len(catalog))
	for _, m := range catalog {
		entry := catalogStatsEntry{MetricName: m.Name}
		// One row per declared source — zero-fill if nothing arrived.
		for _, src := range m.Sources {
			ss := indexed[srcKey{metric: m.Name, source: src.ID}]
			entry.BySource = append(entry.BySource, catalogStatsBySource{
				SourceID:    src.ID,
				Priority:    src.Priority,
				SampleCount: ss.SampleCount,
				LastSeenAt:  formatLastSeen(ss.LastSeenAt),
			})
		}
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, catalogStatsResponse{
		LookbackSeconds: int(catalogStatsLookback.Seconds()),
		Stats:           entries,
	})
}

func (h *MetricsHandler) handleGetMetricSeries(w http.ResponseWriter, r *http.Request) {
	enriched, effective, ok := h.queryEnrichedSamples(w, r)
	if !ok {
		return
	}

	// Group samples by the complete public series identity. This prevents
	// different entities, dimensions, or dynamically resolved units from being
	// presented as a single series.
	groups := map[string][]enrichedSample{}
	for _, es := range enriched {
		key := seriesGroupKey(es)
		groups[key] = append(groups[key], es)
	}

	// Build catalog lookup for metadata.
	catalogMap := make(map[string]metrics.CanonicalMetric)
	for _, m := range metrics.DefaultCatalog() {
		catalogMap[m.Name] = m
	}

	// Build sorted series.
	seriesNames := sortedKeys(groups)
	series := make([]metricSeries, 0, len(seriesNames))
	for _, key := range seriesNames {
		samps := groups[key]
		// samps inherits descending order from queryEnrichedSamples.
		name := samps[0].MetricName
		ms := metricSeries{
			MetricName: name,
			Unit:       samps[0].Unit,
			Samples:    samps,
			Edge:       samps[0].Edge,
		}
		if cm, ok := catalogMap[name]; ok {
			ms.Category = string(cm.Category)
			ms.Description = cm.Description
		}
		series = append(series, ms)
	}

	total := len(series)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(series, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
		"query":  effective,
		"series": page,
		"total":  total,
	}, nextToken)
}

func edgeRefFromLabels(labels map[string]string) *edgeRef {
	if labels == nil {
		return nil
	}
	source := entities.EntityID(labels[semconv.LabelEdgeClientID])
	target := entities.EntityID(labels[semconv.LabelEdgeServerID])
	if source == "" || target == "" {
		return nil
	}
	return &edgeRef{SourceID: source, TargetID: target}
}

func seriesGroupKey(sample enrichedSample) string {
	identity := struct {
		MetricName string            `json:"metric_name"`
		EntityID   entities.EntityID `json:"entity_id"`
		Unit       metrics.Unit      `json:"unit"`
		Labels     map[string]string `json:"labels"`
		Edge       *edgeRef          `json:"edge"`
	}{
		MetricName: sample.MetricName,
		EntityID:   sample.EntityID,
		Unit:       sample.Unit,
		Labels:     sample.Labels,
		Edge:       sample.Edge,
	}
	encoded, _ := json.Marshal(identity)
	return string(encoded)
}
