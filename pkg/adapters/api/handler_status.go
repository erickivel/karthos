package api

import (
	"context"
	"net/http"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
)

// StatusHandler serves system status and discovery profile endpoints.
type StatusHandler struct {
	metadataStore    persistence.MetadataStore
	graphStore       persistence.GraphStore
	metricStore      persistence.MetricStore
	startTime        time.Time
	hasMetricStore   bool
	hasGraphStore    bool
	hasMetadataStore bool
	hasEntityReg     bool
	staleThreshold   time.Duration
	logger           *logging.Logger
}

// RegisterRoutes registers status endpoints on the given mux.
func (h *StatusHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/status", h.handleGetStatus)
	mux.HandleFunc("GET /api/v1/discovery", h.handleGetDiscovery)
}

func (h *StatusHandler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	componentStatus := func(available bool) string {
		if available {
			return "available"
		}
		return "unavailable"
	}

	reasons := []string{}

	// Determine component-level status.
	if !h.hasGraphStore {
		reasons = append(reasons, "graph_store unavailable")
	}
	if !h.hasEntityReg {
		reasons = append(reasons, "entity_registry unavailable")
	}
	if !h.hasMetricStore {
		reasons = append(reasons, "metric_store unavailable")
	}
	if !h.hasMetadataStore {
		reasons = append(reasons, "metadata_store unavailable")
	}

	// Snapshot freshness.
	var lastSnapshotAt *time.Time
	var snapshotFreshness float64
	dataStale := true

	if h.graphStore != nil {
		snap, err := h.graphStore.GetLatest(r.Context())
		if err == nil && snap != nil {
			lastSnapshotAt = &snap.CreatedAt
			snapshotFreshness = time.Since(snap.CreatedAt).Seconds()
			dataStale = snapshotFreshness > h.staleThreshold.Seconds()
			if dataStale {
				reasons = append(reasons, "snapshot stale (>"+h.staleThreshold.String()+")")
			}
		} else if err == nil && snap == nil {
			reasons = append(reasons, "no snapshot available")
		}
	}

	// Discovery timestamp.
	var lastDiscoveryAt *time.Time
	if h.metadataStore != nil {
		prof, err := h.metadataStore.GetDiscovery(r.Context())
		if err == nil && prof != nil {
			lastDiscoveryAt = &prof.DiscoveredAt
		}
	}

	// Determine overall system status.
	systemStatus := "nominal"
	if !h.hasGraphStore || !h.hasEntityReg {
		systemStatus = "disconnected"
	} else if !h.hasMetricStore || !h.hasMetadataStore || dataStale {
		systemStatus = "degraded"
	}

	resp := map[string]any{
		"system_status": systemStatus,
		"reasons":       reasons,
		"version":       schema.CurrentVersion,
		"components": map[string]string{
			"metric_store":    componentStatus(h.hasMetricStore),
			"graph_store":     componentStatus(h.hasGraphStore),
			"metadata_store":  componentStatus(h.hasMetadataStore),
			"entity_registry": componentStatus(h.hasEntityReg),
		},
		"uptime_seconds":             int(time.Since(h.startTime).Seconds()),
		"snapshot_freshness_seconds": snapshotFreshness,
		"data_stale":                 dataStale,
	}
	if lastSnapshotAt != nil {
		resp["last_snapshot_at"] = *lastSnapshotAt
	}
	if lastDiscoveryAt != nil {
		resp["last_discovery_at"] = *lastDiscoveryAt
	}

	// Health summary from metric store.
	if h.metricStore != nil {
		if hs := h.computeHealthSummary(r.Context()); hs != nil {
			resp["health_summary"] = hs
			if hs.Nodes.Unhealthy > 0 {
				if systemStatus == "nominal" {
					systemStatus = "degraded"
					resp["system_status"] = systemStatus
				}
				reasons = append(reasons, "unhealthy nodes detected")
				resp["reasons"] = reasons
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type healthSummary struct {
	Nodes nodeSummary `json:"nodes"`
	Pods  podSummary  `json:"pods"`
}

type nodeSummary struct {
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

type podSummary struct {
	Running int `json:"running"`
	Pending int `json:"pending"`
	Failed  int `json:"failed"`
}

func (h *StatusHandler) computeHealthSummary(ctx context.Context) *healthSummary {
	now := time.Now().UTC()
	from := now.Add(-5 * time.Minute)

	hs := &healthSummary{}

	// Node health from node_condition_ready metric.
	nodeSamples, err := h.metricStore.Query(ctx, query.MetricQuery{
		MetricName: "node_condition_ready",
		From:       from,
		To:         now,
	})
	if err == nil {
		// Deduplicate: keep latest sample per entity.
		latest := make(map[entities.EntityID]float64)
		latestTS := make(map[entities.EntityID]time.Time)
		for _, s := range nodeSamples {
			if ts, ok := latestTS[s.EntityID]; !ok || s.Timestamp.After(ts) {
				latest[s.EntityID] = s.Value
				latestTS[s.EntityID] = s.Timestamp
			}
		}
		for _, v := range latest {
			if v >= 1.0 {
				hs.Nodes.Healthy++
			} else {
				hs.Nodes.Unhealthy++
			}
		}
	}

	// Pod phase from pod_phase metric.
	podSamples, err := h.metricStore.Query(ctx, query.MetricQuery{
		MetricName: "pod_phase",
		From:       from,
		To:         now,
	})
	if err == nil {
		latest := make(map[entities.EntityID]float64)
		latestTS := make(map[entities.EntityID]time.Time)
		for _, s := range podSamples {
			if ts, ok := latestTS[s.EntityID]; !ok || s.Timestamp.After(ts) {
				latest[s.EntityID] = s.Value
				latestTS[s.EntityID] = s.Timestamp
			}
		}
		for _, v := range latest {
			switch int(v) {
			case 1:
				hs.Pods.Running++
			case 2:
				hs.Pods.Pending++
			case 4:
				hs.Pods.Failed++
			}
		}
	}

	return hs
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
