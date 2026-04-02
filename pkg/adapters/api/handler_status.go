package api

import (
	"net/http"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
)

// StatusHandler serves system status and discovery profile endpoints.
type StatusHandler struct {
	metadataStore    persistence.MetadataStore
	startTime        time.Time
	hasMetricStore   bool
	hasGraphStore    bool
	hasMetadataStore bool
	hasEntityReg     bool
	logger           *logging.Logger
}

// RegisterRoutes registers status endpoints on the given mux.
func (h *StatusHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/status", h.handleGetStatus)
	mux.HandleFunc("GET /api/v1/discovery", h.handleGetDiscovery)
}

func (h *StatusHandler) handleGetStatus(w http.ResponseWriter, _ *http.Request) {
	componentStatus := func(available bool) string {
		if available {
			return "available"
		}
		return "unavailable"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": schema.CurrentVersion,
		"components": map[string]string{
			"metric_store":    componentStatus(h.hasMetricStore),
			"graph_store":     componentStatus(h.hasGraphStore),
			"metadata_store":  componentStatus(h.hasMetadataStore),
			"entity_registry": componentStatus(h.hasEntityReg),
		},
		"uptime_seconds": int(time.Since(h.startTime).Seconds()),
	})
}

func (h *StatusHandler) handleGetDiscovery(w http.ResponseWriter, r *http.Request) {
	if h.metadataStore == nil {
		writeError(w, http.StatusNotFound, "metadata store not configured")
		return
	}

	prof, err := h.metadataStore.GetDiscovery(r.Context())
	if err != nil {
		h.logger.Warn("failed to get discovery profile", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve discovery profile")
		return
	}
	if prof == nil {
		writeError(w, http.StatusNotFound, "no discovery profile available")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cluster_id":            prof.ClusterID,
		"cluster_name":          prof.ClusterName,
		"discovered_at":         prof.DiscoveredAt,
		"node_count":            len(prof.Nodes),
		"namespace_count":       len(prof.Namespaces),
		"workload_count":        len(prof.Workloads),
		"service_count":         len(prof.Services),
		"ingress_count":         len(prof.Ingresses),
		"sources":               prof.Sources,
		"collection_paths":      prof.CollectionPaths,
		"recommended_receivers": prof.RecommendedReceivers,
	})
}
