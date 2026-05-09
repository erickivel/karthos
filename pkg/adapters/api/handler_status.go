package api

import (
	"context"
	"net/http"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
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

// podSummary buckets pods by their oksg.pod.health.status string label.
// Field names match the public string set documented at
// pkg/core/metrics/health_status.go and in docs/api-reference.md.
type podSummary struct {
	Healthy         int `json:"healthy"`
	RunningNotReady int `json:"running_not_ready"`
	Restarting      int `json:"restarting"`
	Pending         int `json:"pending"`
	Succeeded       int `json:"succeeded"`
	Failed          int `json:"failed"`
	Unknown         int `json:"unknown"`
}

func (h *StatusHandler) computeHealthSummary(ctx context.Context) *healthSummary {
	now := time.Now().UTC()
	from := now.Add(-5 * time.Minute)

	hs := &healthSummary{}

	// Node health from k8s.node.condition.ready.
	nodeSamples, err := h.metricStore.Query(ctx, query.MetricQuery{
		MetricName: "k8s.node.condition.ready",
		From:       from,
		To:         now,
	})
	if err == nil {
		for _, v := range latestSamplePerEntity(nodeSamples) {
			if v >= 1.0 {
				hs.Nodes.Healthy++
			} else {
				hs.Nodes.Unhealthy++
			}
		}
	}

	// Pod health from the derived oksg.pod.health.status enum (string label
	// already populated on each sample by the metric store).
	if podIDs := h.podEntityIDs(ctx); len(podIDs) > 0 {
		podSamples, err := h.metricStore.Query(ctx, query.MetricQuery{
			MetricName: "oksg.pod.health.status",
			EntityIDs:  podIDs,
			From:       from,
			To:         now,
		})
		if err == nil {
			latestLabel := latestLabelPerEntity(podSamples)
			for _, id := range podIDs {
				switch latestLabel[id] {
				case metrics.HealthHealthy:
					hs.Pods.Healthy++
				case metrics.HealthRunningNotReady:
					hs.Pods.RunningNotReady++
				case metrics.HealthRestarting:
					hs.Pods.Restarting++
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
		} else {
			h.logger.Warn("pod health-status query failed", logging.Error(err))
			hs.Pods.Unknown += len(podIDs)
		}
	}

	return hs
}

// podEntityIDs returns the IDs of every Pod in the latest topology snapshot.
// Used as the multi-match filter on the health-status query so the metric
// store only returns samples for currently-known pods.
func (h *StatusHandler) podEntityIDs(ctx context.Context) []entities.EntityID {
	if h.graphStore == nil {
		return nil
	}
	snap, err := h.graphStore.GetLatest(ctx)
	if err != nil || snap == nil {
		return nil
	}
	var ids []entities.EntityID
	for _, node := range snap.Graph.Nodes {
		if node.Kind == entities.KindPod {
			ids = append(ids, node.ID)
		}
	}
	return ids
}

// latestLabelPerEntity collapses a list of MetricSamples to one ValueLabel per
// entity, keeping the most recent timestamp.
func latestLabelPerEntity(samples []metrics.MetricSample) map[entities.EntityID]string {
	latest := make(map[entities.EntityID]string, len(samples))
	latestTS := make(map[entities.EntityID]time.Time, len(samples))
	for _, sample := range samples {
		if ts, ok := latestTS[sample.EntityID]; !ok || sample.Timestamp.After(ts) {
			latest[sample.EntityID] = sample.ValueLabel
			latestTS[sample.EntityID] = sample.Timestamp
		}
	}
	return latest
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
