package api

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// GraphHandler serves topology graph query endpoints.
type GraphHandler struct {
	graphStore     persistence.GraphStore
	entityRegistry persistence.EntityRegistry
	metricStore    persistence.MetricStore
	logger         *logging.Logger
}

// RegisterRoutes registers graph endpoints on the given mux.
func (h *GraphHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/graph", h.handleGetGraph)
	mux.HandleFunc("GET /api/v1/graph/nodes", h.handleListNodes)
	mux.HandleFunc("GET /api/v1/graph/nodes/{id}", h.handleGetNode)
	mux.HandleFunc("GET /api/v1/graph/edges", h.handleListEdges)
}

// --- response types ---

type enrichedNode struct {
	ID        entities.EntityID   `json:"id"`
	Kind      entities.EntityKind `json:"kind"`
	Name      string              `json:"name,omitempty"`
	Namespace string              `json:"namespace,omitempty"`
	Cluster   string              `json:"cluster,omitempty"`
	Status    string              `json:"status"`
	Labels    map[string]string   `json:"labels,omitempty"`
}

type relatedEntitySummary struct {
	ID        entities.EntityID   `json:"id"`
	Kind      entities.EntityKind `json:"kind"`
	Name      string              `json:"name,omitempty"`
	Namespace string              `json:"namespace,omitempty"`
	Cluster   string              `json:"cluster,omitempty"`
	Labels    map[string]string   `json:"labels,omitempty"`
}

type enrichedEdge struct {
	ID          string                `json:"id"`
	Source      entities.EntityID     `json:"source"`
	Target      entities.EntityID     `json:"target"`
	Relation    topology.RelationType `json:"relation"`
	Confidence  float64               `json:"confidence"`
	Provenance  []string              `json:"provenance,omitempty"`
	FirstSeen   time.Time             `json:"first_seen"`
	LastSeen    time.Time             `json:"last_seen"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
	RelatedNode *relatedEntitySummary `json:"related_node,omitempty"`
}

type availableFilters struct {
	Kinds      []string `json:"kinds"`
	Namespaces []string `json:"namespaces"`
	Relations  []string `json:"relations"`
}

// --- helpers ---

// enrichNodeFrom builds an enrichedNode from a topology node, a pre-resolved
// entity (may be nil if the registry lookup missed), and a pre-resolved
// status string from the latest health-status sample (empty if none).
// The status falls back to metrics.HealthUnknown so the field is always
// populated with one of the public string-set values.
func enrichNodeFrom(n topology.Node, entity entities.Entity, status string) enrichedNode {
	if status == "" {
		status = metrics.HealthUnknown
	}
	en := enrichedNode{
		ID:     n.ID,
		Kind:   n.Kind,
		Status: status,
		Labels: n.Labels,
	}
	if entity != nil {
		ref := entityRefOf(entity)
		en.Name = ref.Name
		en.Namespace = ref.Namespace
		en.Cluster = ref.Cluster
	}
	if en.Name == "" && n.Labels != nil {
		en.Name = n.Labels["name"]
	}
	if en.Namespace == "" && n.Labels != nil {
		en.Namespace = n.Labels["namespace"]
	}
	return en
}

// batchEntities resolves the given IDs through the registry in a single call.
// Returns an empty map if the registry is nil or ids is empty.
func (h *GraphHandler) batchEntities(ctx context.Context, ids []entities.EntityID) map[entities.EntityID]entities.Entity {
	if h.entityRegistry == nil || len(ids) == 0 {
		return map[entities.EntityID]entities.Entity{}
	}
	resolved, err := h.entityRegistry.GetEntities(ctx, ids)
	if err != nil {
		h.logger.Warn("batch entity lookup failed", logging.Error(err))
		return map[entities.EntityID]entities.Entity{}
	}
	return resolved
}

// resolveHealthStatus issues at most two metric-store queries (one per
// derived health-status metric) for the supplied node IDs and returns a map
// of entity ID to its public string label. Missing entries (no recent
// sample, no metric store, query error) are absent from the returned map;
// the caller defaults to metrics.HealthUnknown.
//
// The lookup is per-snapshot (not per-node) and uses the multi-match
// entity_id=~"..." filter that metricSelector already supports, so this is
// O(2 queries) regardless of node count.
func (h *GraphHandler) resolveHealthStatus(ctx context.Context, nodes []topology.Node) map[entities.EntityID]string {
	if h.metricStore == nil || len(nodes) == 0 {
		return nil
	}
	var podIDs, containerIDs []entities.EntityID
	for _, n := range nodes {
		switch n.Kind {
		case entities.KindPod:
			podIDs = append(podIDs, n.ID)
		case entities.KindContainer:
			containerIDs = append(containerIDs, n.ID)
		}
	}
	resolved := make(map[entities.EntityID]string)
	h.fillHealthStatus(ctx, "oksg.pod.health.status", podIDs, resolved)
	h.fillHealthStatus(ctx, "oksg.container.health.status", containerIDs, resolved)
	return resolved
}

func (h *GraphHandler) fillHealthStatus(ctx context.Context, metricName string, ids []entities.EntityID, into map[entities.EntityID]string) {
	if len(ids) == 0 {
		return
	}
	now := time.Now()
	samples, err := h.metricStore.Query(ctx, query.MetricQuery{
		MetricName: metricName,
		EntityIDs:  ids,
		From:       now.Add(-5 * time.Minute),
		To:         now,
	})
	if err != nil {
		h.logger.Warn("health-status query failed", logging.Error(err), logging.String("metric", metricName))
		return
	}
	// Keep the most recent sample per entity (samples come back in
	// descending time order from the store).
	for _, s := range samples {
		if _, seen := into[s.EntityID]; seen {
			continue
		}
		if s.ValueLabel == "" {
			continue
		}
		into[s.EntityID] = s.ValueLabel
	}
}

func summaryFrom(entity entities.Entity) *relatedEntitySummary {
	if entity == nil {
		return nil
	}
	ref := entityRefOf(entity)
	return &relatedEntitySummary{
		ID:        entity.ID(),
		Kind:      entity.Kind(),
		Name:      ref.Name,
		Namespace: ref.Namespace,
		Cluster:   ref.Cluster,
		Labels:    entity.Labels(),
	}
}

func toEnrichedEdge(e topology.Edge, related *relatedEntitySummary) enrichedEdge {
	prov := e.Provenance
	if prov == nil {
		prov = []string{}
	}
	return enrichedEdge{
		ID:          e.ID,
		Source:      e.Source,
		Target:      e.Target,
		Relation:    e.Relation,
		Confidence:  e.Confidence,
		Provenance:  prov,
		FirstSeen:   e.FirstSeen,
		LastSeen:    e.LastSeen,
		Metadata:    e.Metadata,
		RelatedNode: related,
	}
}

type entityRef struct {
	Name      string
	Namespace string
	Cluster   string
}

func entityRefOf(e entities.Entity) entityRef {
	switch se := e.(type) {
	case persistence.StoredEntity:
		return entityRef{Name: se.Name, Namespace: se.Namespace, Cluster: se.Cluster}
	case *persistence.StoredEntity:
		return entityRef{Name: se.Name, Namespace: se.Namespace, Cluster: se.Cluster}
	}
	var ref entityRef
	if labels := e.Labels(); labels != nil {
		ref.Name = labels["name"]
		ref.Namespace = labels["namespace"]
	}
	return ref
}

// --- handlers ---

func (h *GraphHandler) handleGetGraph(w http.ResponseWriter, r *http.Request) {
	if h.graphStore == nil {
		writeError(w, http.StatusNotFound, "graph store not configured")
		return
	}

	snap, err := h.graphStore.GetLatest(r.Context())
	if err != nil {
		h.logger.Warn("failed to get latest graph", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve graph")
		return
	}
	if snap == nil {
		writeError(w, http.StatusNotFound, "no graph snapshot available")
		return
	}

	nodes, edges := canonicalizeGraph(snap.Graph.Nodes, snap.Graph.Edges)

	kindCounts := map[string]int{}
	nsSet := map[string]struct{}{}
	for _, n := range nodes {
		kindCounts[string(n.Kind)]++
		if ns, ok := n.Labels["namespace"]; ok && ns != "" {
			nsSet[ns] = struct{}{}
		}
	}

	relationCounts := map[string]int{}
	relSet := map[string]struct{}{}
	for _, e := range edges {
		relationCounts[string(e.Relation)]++
		relSet[string(e.Relation)] = struct{}{}
	}

	kinds := sortedKeys(kindCounts)
	namespaces := sortedSetKeys(nsSet)
	relations := sortedKeys(relationCounts)

	ids := make([]entities.EntityID, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	resolved := h.batchEntities(r.Context(), ids)
	healthByID := h.resolveHealthStatus(r.Context(), nodes)

	enrichedNodes := make([]enrichedNode, 0, len(nodes))
	for _, n := range nodes {
		enrichedNodes = append(enrichedNodes, enrichNodeFrom(n, resolved[n.ID], healthByID[n.ID]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":    snap.Version,
		"from":       snap.From,
		"to":         snap.To,
		"created_at": snap.CreatedAt,
		"graph": map[string]any{
			"timestamp": snap.Graph.Timestamp,
			"nodes":     enrichedNodes,
			"edges":     edges,
		},
		"summary": map[string]any{
			"node_count":           len(nodes),
			"edge_count":           len(edges),
			"kind_counts":          kindCounts,
			"relation_counts":      relationCounts,
			"snapshot_age_seconds": time.Since(snap.CreatedAt).Seconds(),
			"available_filters": availableFilters{
				Kinds:      kinds,
				Namespaces: namespaces,
				Relations:  relations,
			},
		},
	})
}

func (h *GraphHandler) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if h.graphStore == nil {
		writeError(w, http.StatusNotFound, "graph store not configured")
		return
	}

	snap, err := h.graphStore.GetLatest(r.Context())
	if err != nil {
		h.logger.Warn("failed to get latest graph", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve graph")
		return
	}
	if snap == nil {
		writeJSONPaginated(w, http.StatusOK, map[string]any{"nodes": []enrichedNode{}, "total": 0}, "")
		return
	}

	nodes, _ := canonicalizeGraph(snap.Graph.Nodes, snap.Graph.Edges)

	kindFilter := entities.EntityKind(r.URL.Query().Get("kind"))
	nsFilter := r.URL.Query().Get("namespace")

	var candidates []topology.Node
	for _, n := range nodes {
		if kindFilter != "" && n.Kind != kindFilter {
			continue
		}
		candidates = append(candidates, n)
	}
	candidateIDs := make([]entities.EntityID, 0, len(candidates))
	for _, n := range candidates {
		candidateIDs = append(candidateIDs, n.ID)
	}
	resolved := h.batchEntities(r.Context(), candidateIDs)
	healthByID := h.resolveHealthStatus(r.Context(), candidates)

	var filtered []enrichedNode
	for _, n := range candidates {
		en := enrichNodeFrom(n, resolved[n.ID], healthByID[n.ID])
		if nsFilter != "" && en.Namespace != nsFilter {
			continue
		}
		filtered = append(filtered, en)
	}
	if filtered == nil {
		filtered = []enrichedNode{}
	}

	slices.SortFunc(filtered, func(a, b enrichedNode) int {
		if c := cmp.Compare(string(a.Kind), string(b.Kind)); c != 0 {
			return c
		}
		return cmp.Compare(string(a.ID), string(b.ID))
	})

	total := len(filtered)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(filtered, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
		"nodes": page,
		"total": total,
	}, nextToken)
}

func (h *GraphHandler) handleGetNode(w http.ResponseWriter, r *http.Request) {
	if h.graphStore == nil {
		writeError(w, http.StatusNotFound, "graph store not configured")
		return
	}

	nodeID := entities.EntityID(r.PathValue("id"))

	snap, err := h.graphStore.GetLatest(r.Context())
	if err != nil {
		h.logger.Warn("failed to get latest graph", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve graph")
		return
	}
	if snap == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	nodes, edges := canonicalizeGraph(snap.Graph.Nodes, snap.Graph.Edges)

	var found *topology.Node
	for i := range nodes {
		if nodes[i].ID == nodeID {
			found = &nodes[i]
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	var outbound []topology.Edge
	var inbound []topology.Edge
	for _, edge := range edges {
		if edge.Source == nodeID {
			outbound = append(outbound, edge)
		}
		if edge.Target == nodeID {
			inbound = append(inbound, edge)
		}
	}

	idSet := map[entities.EntityID]struct{}{nodeID: {}}
	for _, e := range outbound {
		idSet[e.Target] = struct{}{}
	}
	for _, e := range inbound {
		idSet[e.Source] = struct{}{}
	}
	ids := make([]entities.EntityID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	resolved := h.batchEntities(r.Context(), ids)
	healthByID := h.resolveHealthStatus(r.Context(), []topology.Node{*found})

	enriched := enrichNodeFrom(*found, resolved[nodeID], healthByID[nodeID])

	outboundEnriched := make([]enrichedEdge, 0, len(outbound))
	for _, e := range outbound {
		outboundEnriched = append(outboundEnriched, toEnrichedEdge(e, summaryFrom(resolved[e.Target])))
	}
	inboundEnriched := make([]enrichedEdge, 0, len(inbound))
	for _, e := range inbound {
		inboundEnriched = append(inboundEnriched, toEnrichedEdge(e, summaryFrom(resolved[e.Source])))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"node": enriched,
		"edges": map[string]any{
			"outbound": outboundEnriched,
			"inbound":  inboundEnriched,
		},
	})
}

func (h *GraphHandler) handleListEdges(w http.ResponseWriter, r *http.Request) {
	if h.graphStore == nil {
		writeError(w, http.StatusNotFound, "graph store not configured")
		return
	}

	sourceFilter := entities.EntityID(r.URL.Query().Get("source"))
	targetFilter := entities.EntityID(r.URL.Query().Get("target"))
	relationFilter := topology.RelationType(r.URL.Query().Get("relation"))

	edges, err := h.graphStore.QueryEdges(r.Context(), query.EdgeQuery{})
	if err != nil {
		h.logger.Warn("failed to query edges", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query edges")
		return
	}

	if snap, snapErr := h.graphStore.GetLatest(r.Context()); snapErr == nil && snap != nil {
		_, edges = canonicalizeGraph(snap.Graph.Nodes, edges)
	}

	var filtered []topology.Edge
	for _, e := range edges {
		if sourceFilter != "" && e.Source != sourceFilter {
			continue
		}
		if targetFilter != "" && e.Target != targetFilter {
			continue
		}
		if relationFilter != "" && e.Relation != relationFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	edges = filtered

	if edges == nil {
		edges = []topology.Edge{}
	}

	slices.SortFunc(edges, func(a, b topology.Edge) int {
		if c := cmp.Compare(string(a.Relation), string(b.Relation)); c != 0 {
			return c
		}
		return cmp.Compare(string(a.Source), string(b.Source))
	})

	total := len(edges)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(edges, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
		"edges": page,
		"total": total,
	}, nextToken)
}

// --- utility ---

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedSetKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
