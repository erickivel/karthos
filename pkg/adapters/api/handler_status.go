package api

import (
	"context"
	"net/http"
	"time"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/logging"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/topology"
)

// StatusHandler serves system status and capability profile endpoints.
type StatusHandler struct {
	metadataStore    persistence.MetadataStore
	graphStore       persistence.GraphStore
	metricStore      persistence.MetricStore
	healthResolver   *HealthResolver
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
	var snapshot *topology.Snapshot
	var snapshotFreshness float64
	dataStale := true
	graphReadFailed := false

	if h.graphStore != nil {
		snap, err := h.graphStore.GetLatest(r.Context())
		if err == nil && snap != nil {
			snapshot = snap
			lastSnapshotAt = &snap.CreatedAt
			snapshotFreshness = time.Since(snap.CreatedAt).Seconds()
			dataStale = snapshotFreshness > h.staleThreshold.Seconds()
			if dataStale {
				reasons = append(reasons, "snapshot stale (>"+h.staleThreshold.String()+")")
			}
		} else if err == nil && snap == nil {
			reasons = append(reasons, "no snapshot available")
		} else {
			graphReadFailed = true
			reasons = append(reasons, "failed to read latest graph")
			h.logger.Warn("failed to read latest graph for status", logging.Error(err))
		}
	}

	// Discovery timestamp.
	var lastDiscoveryAt *time.Time
	metadataReadFailed := false
	if h.metadataStore != nil {
		prof, err := h.metadataStore.GetDiscovery(r.Context())
		if err == nil && prof != nil {
			lastDiscoveryAt = &prof.DiscoveredAt
		} else if err != nil {
			metadataReadFailed = true
			reasons = append(reasons, "failed to read capability profile")
			h.logger.Warn("failed to read capability profile for status", logging.Error(err))
		}
	}

	// Determine overall system status.
	systemStatus := "nominal"
	if !h.hasGraphStore || !h.hasEntityReg {
		systemStatus = "disconnected"
	} else if !h.hasMetricStore || !h.hasMetadataStore || dataStale || graphReadFailed || metadataReadFailed {
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

	// Health summary is derived from current graph entities. A missing or
	// failing metric store is represented explicitly instead of masquerading as
	// legitimate unknown telemetry.
	if snapshot != nil {
		hs := h.computeHealthSummary(r.Context(), snapshot.Graph.Nodes)
		resp["health_summary"] = hs
		if hs.Nodes.Unhealthy > 0 {
			if systemStatus == "nominal" {
				systemStatus = "degraded"
				resp["system_status"] = systemStatus
			}
			reasons = append(reasons, "unhealthy nodes detected")
		}
		if hs.Nodes.Status == healthQualityUnavailable {
			reasons = append(reasons, "node health summary unavailable")
		}
		if hs.Pods.Status == healthQualityUnavailable {
			reasons = append(reasons, "pod health summary unavailable")
		}
		if hs.Status != healthQualityComplete && systemStatus == "nominal" {
			systemStatus = "degraded"
			resp["system_status"] = systemStatus
		}
		resp["reasons"] = reasons
	}

	writeJSON(w, http.StatusOK, resp)
}

type healthSummary struct {
	Status healthQuality `json:"status"`
	Nodes  nodeSummary   `json:"nodes"`
	Pods   podSummary    `json:"pods"`
}

type nodeSummary struct {
	Status    healthQuality `json:"status"`
	Healthy   int           `json:"healthy"`
	Unhealthy int           `json:"unhealthy"`
	Unknown   int           `json:"unknown"`
}

// podSummary buckets pods by their direct pod-phase-derived string label.
type podSummary struct {
	Status    healthQuality `json:"status"`
	Healthy   int           `json:"healthy"`
	Pending   int           `json:"pending"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
	Unknown   int           `json:"unknown"`
}

func (h *StatusHandler) computeHealthSummary(ctx context.Context, graphNodes []topology.Node) *healthSummary {
	var nodeIDs, podIDs []entities.EntityID
	podNodes := make([]topology.Node, 0)
	for _, node := range graphNodes {
		switch node.Kind {
		case entities.KindNode:
			nodeIDs = append(nodeIDs, node.ID)
		case entities.KindPod:
			podIDs = append(podIDs, node.ID)
			podNodes = append(podNodes, node)
		}
	}
	if h.metricStore == nil {
		return &healthSummary{
			Status: healthQualityUnavailable,
			Nodes:  nodeSummary{Status: healthQualityUnavailable, Unknown: len(nodeIDs)},
			Pods:   podSummary{Status: healthQualityUnavailable, Unknown: len(podIDs)},
		}
	}

	nodeResult := queryHealthSamples(ctx, h.metricStore, "k8s.node.condition.ready", nodeIDs, false, time.Now().UTC())
	podResolution := h.healthResolver.Resolve(ctx, podNodes, false)
	podResult := healthSampleResult{quality: podResolution.Pods}
	if len(podResolution.Errors) > 0 {
		podResult.err = podResolution.Errors[0]
	}
	applicable := make([]healthQuality, 0, 2)
	if len(nodeIDs) > 0 {
		applicable = append(applicable, nodeResult.quality)
	}
	if len(podIDs) > 0 {
		applicable = append(applicable, podResult.quality)
	}
	overall := healthQualityComplete
	if len(applicable) > 0 {
		overall = combineHealthQuality(applicable...)
	}
	hs := &healthSummary{
		Status: overall,
		Nodes:  nodeSummary{Status: nodeResult.quality},
		Pods:   podSummary{Status: podResult.quality},
	}

	if nodeResult.err != nil {
		h.logger.Warn("node health-status query failed", logging.Error(nodeResult.err))
	}
	if nodeResult.quality == healthQualityUnavailable {
		hs.Nodes.Unknown = len(nodeIDs)
	} else {
		latest := latestSamplePerEntity(nodeResult.samples)
		for _, id := range nodeIDs {
			value, ok := latest[id]
			if !ok {
				hs.Nodes.Unknown++
			} else if value >= 1.0 {
				hs.Nodes.Healthy++
			} else {
				hs.Nodes.Unhealthy++
			}
		}
	}

	if podResult.err != nil {
		h.logger.Warn("pod health-status query failed", logging.Error(podResult.err))
	}
	if podResult.quality == healthQualityUnavailable {
		hs.Pods.Unknown = len(podIDs)
	} else {
		for _, id := range podIDs {
			switch podResolution.Statuses[id] {
			case metrics.HealthHealthy:
				hs.Pods.Healthy++
			case metrics.HealthPending:
				hs.Pods.Pending++
			case metrics.HealthSucceeded:
				hs.Pods.Succeeded++
			case metrics.HealthFailed:
				hs.Pods.Failed++
			default:
				hs.Pods.Unknown++
			}
		}
	}

	return hs
}

func latestSamplePerEntity(samples []metrics.MetricSample) map[entities.EntityID]float64 {
	latest := make(map[entities.EntityID]float64, len(samples))
	latestTS := make(map[entities.EntityID]time.Time, len(samples))
	for _, sample := range samples {
		if ts, ok := latestTS[sample.EntityID]; !ok || sample.Timestamp.After(ts) {
			latest[sample.EntityID] = sample.Value
			latestTS[sample.EntityID] = sample.Timestamp
		}
	}
	return latest
}

func (h *StatusHandler) handleGetDiscovery(w http.ResponseWriter, r *http.Request) {
	if h.metadataStore == nil {
		writeError(w, http.StatusNotFound, "metadata store not configured")
		return
	}

	prof, err := h.metadataStore.GetDiscovery(r.Context())
	if err != nil {
		h.logger.Warn("failed to get capability profile", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve capability profile")
		return
	}
	if prof == nil {
		writeError(w, http.StatusNotFound, "no capability profile available")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"cluster_id":    prof.ClusterID,
		"cluster_name":  prof.ClusterName,
		"discovered_at": prof.DiscoveredAt,
		"capabilities":  prof.Capabilities,
	})
}
