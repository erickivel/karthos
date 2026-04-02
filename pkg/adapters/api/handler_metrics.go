package api

import (
	"net/http"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

// MetricsHandler serves metric query and catalog endpoints.
type MetricsHandler struct {
	metricStore persistence.MetricStore
	logger      *logging.Logger
}

// RegisterRoutes registers metrics endpoints on the given mux.
func (h *MetricsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/metrics", h.handleQueryMetrics)
	mux.HandleFunc("GET /api/v1/metrics/catalog", h.handleGetCatalog)
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

	var entityIDs []entities.EntityID
	for _, id := range r.URL.Query()["entity_id"] {
		entityIDs = append(entityIDs, entities.EntityID(id))
	}

	q := query.MetricQuery{
		EntityIDs:  entityIDs,
		MetricName: r.URL.Query().Get("metric_name"),
		From:       from,
		To:         to,
	}

	samples, err := h.metricStore.Query(r.Context(), q)
	if err != nil {
		h.logger.Warn("failed to query metrics", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query metrics")
		return
	}
	if samples == nil {
		samples = []metrics.MetricSample{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"samples": samples,
		"total":   len(samples),
	})
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
