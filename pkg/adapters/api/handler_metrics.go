package api

import (
	"cmp"
	"net/http"
	"slices"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

// MetricsHandler serves metric query and catalog endpoints.
type MetricsHandler struct {
	metricStore persistence.MetricStore
	unitLookup  map[string]metrics.Unit
	logger      *logging.Logger
}

// newMetricsHandler creates a MetricsHandler with a prebuilt unit lookup from the catalog.
func newMetricsHandler(store persistence.MetricStore, logger *logging.Logger) *MetricsHandler {
	lookup := make(map[string]metrics.Unit)
	for _, m := range metrics.DefaultCatalog() {
		lookup[m.Name] = m.Unit
	}
	return &MetricsHandler{
		metricStore: store,
		unitLookup:  lookup,
		logger:      logger,
	}
}

// RegisterRoutes registers metrics endpoints on the given mux.
func (h *MetricsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/metrics", h.handleQueryMetrics)
	mux.HandleFunc("GET /api/v1/metrics/catalog", h.handleGetCatalog)
	mux.HandleFunc("GET /api/v1/metrics/series", h.handleGetMetricSeries)
}

type enrichedSample struct {
	MetricName string            `json:"metric_name"`
	EntityID   entities.EntityID `json:"entity_id"`
	Value      float64           `json:"value"`
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

func (h *MetricsHandler) handleQueryMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metricStore == nil {
		writeError(w, http.StatusNotFound, "metric store not configured")
		return
	}

	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' parameter: expected RFC3339 format")
		return
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' parameter: expected RFC3339 format")
		return
	}

	metricName := r.URL.Query().Get("metric_name")
	if metricName == "" {
		writeError(w, http.StatusBadRequest, "metric_name query parameter is required")
		return
	}

	var entityIDs []entities.EntityID
	for _, id := range r.URL.Query()["entity_id"] {
		entityIDs = append(entityIDs, entities.EntityID(id))
	}

	q := query.MetricQuery{
		EntityIDs:    entityIDs,
		MetricName:   metricName,
		From:         from,
		To:           to,
		EdgeSourceID: entities.EntityID(r.URL.Query().Get("edge_source_id")),
		EdgeTargetID: entities.EntityID(r.URL.Query().Get("edge_target_id")),
	}

	samples, err := h.metricStore.Query(r.Context(), q)
	if err != nil {
		h.logger.Warn("failed to query metrics", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query metrics")
		return
	}

	enriched := make([]enrichedSample, 0, len(samples))
	for _, s := range samples {
		enriched = append(enriched, enrichedSample{
			MetricName: s.MetricName,
			EntityID:   s.EntityID,
			Value:      s.Value,
			Timestamp:  s.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			Labels:     s.Labels,
			Unit:       h.unitLookup[s.MetricName],
			Edge:       edgeRefFromLabels(s.Labels),
		})
	}

	slices.SortFunc(enriched, func(a, b enrichedSample) int {
		return cmp.Compare(b.Timestamp, a.Timestamp) // descending
	})

	total := len(enriched)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(enriched, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
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

func (h *MetricsHandler) handleGetMetricSeries(w http.ResponseWriter, r *http.Request) {
	if h.metricStore == nil {
		writeError(w, http.StatusNotFound, "metric store not configured")
		return
	}

	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'from' parameter: expected RFC3339 format")
		return
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'to' parameter: expected RFC3339 format")
		return
	}

	metricName := r.URL.Query().Get("metric_name")
	if metricName == "" {
		writeError(w, http.StatusBadRequest, "metric_name query parameter is required")
		return
	}

	var entityIDs []entities.EntityID
	for _, id := range r.URL.Query()["entity_id"] {
		entityIDs = append(entityIDs, entities.EntityID(id))
	}

	q := query.MetricQuery{
		EntityIDs:    entityIDs,
		MetricName:   metricName,
		From:         from,
		To:           to,
		EdgeSourceID: entities.EntityID(r.URL.Query().Get("edge_source_id")),
		EdgeTargetID: entities.EntityID(r.URL.Query().Get("edge_target_id")),
	}

	samples, err := h.metricStore.Query(r.Context(), q)
	if err != nil {
		h.logger.Warn("failed to query metrics", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query metrics")
		return
	}

	// Group samples by metric name and edge identity when present.
	groups := map[string][]enrichedSample{}
	for _, s := range samples {
		es := enrichedSample{
			MetricName: s.MetricName,
			EntityID:   s.EntityID,
			Value:      s.Value,
			Timestamp:  s.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
			Labels:     s.Labels,
			Unit:       h.unitLookup[s.MetricName],
			Edge:       edgeRefFromLabels(s.Labels),
		}
		groups[seriesGroupKey(s.MetricName, es.Edge)] = append(groups[seriesGroupKey(s.MetricName, es.Edge)], es)
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
		slices.SortFunc(samps, func(a, b enrichedSample) int {
			return cmp.Compare(b.Timestamp, a.Timestamp) // descending
		})

		name := samps[0].MetricName
		ms := metricSeries{
			MetricName: name,
			Unit:       h.unitLookup[name],
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
		"series": page,
		"total":  total,
	}, nextToken)
}

func edgeRefFromLabels(labels map[string]string) *edgeRef {
	if labels == nil {
		return nil
	}
	source := entities.EntityID(labels["edge_client_id"])
	target := entities.EntityID(labels["edge_server_id"])
	if source == "" || target == "" {
		return nil
	}
	return &edgeRef{SourceID: source, TargetID: target}
}

func seriesGroupKey(metricName string, edge *edgeRef) string {
	if edge == nil {
		return metricName
	}
	return metricName + "|" + string(edge.SourceID) + "|" + string(edge.TargetID)
}
