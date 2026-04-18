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
	"github.com/erickivel/oksg/pkg/core/topology"
)

// GraphHandler serves topology graph query endpoints.
type GraphHandler struct {
	graphStore     persistence.GraphStore
	entityRegistry persistence.EntityRegistry
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

func (h *GraphHandler) enrichNode(ctx context.Context, n topology.Node) enrichedNode {
	en := enrichedNode{
		ID:     n.ID,
		Kind:   n.Kind,
		Status: "unknown",
		Labels: n.Labels,
	}
	if h.entityRegistry != nil {
		if entity, err := h.entityRegistry.GetEntity(ctx, n.ID); err == nil {
			ref := entityRefOf(entity)
			en.Name = ref.Name
			en.Namespace = ref.Namespace
			en.Cluster = ref.Cluster
		}
	}
	if en.Name == "" && n.Labels != nil {
		en.Name = n.Labels["name"]
	}
	if en.Namespace == "" && n.Labels != nil {
		en.Namespace = n.Labels["namespace"]
	}
	return en
}

func (h *GraphHandler) resolveEntitySummary(ctx context.Context, id entities.EntityID) *relatedEntitySummary {
	if h.entityRegistry == nil {
		return nil
	}
	entity, err := h.entityRegistry.GetEntity(ctx, id)
	if err != nil {
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

	kindCounts := map[string]int{}
	nsSet := map[string]struct{}{}
	for _, n := range snap.Graph.Nodes {
		kindCounts[string(n.Kind)]++
		if ns, ok := n.Labels["namespace"]; ok && ns != "" {
			nsSet[ns] = struct{}{}
		}
	}

	relationCounts := map[string]int{}
	relSet := map[string]struct{}{}
	for _, e := range snap.Graph.Edges {
		relationCounts[string(e.Relation)]++
		relSet[string(e.Relation)] = struct{}{}
	}

	kinds := sortedKeys(kindCounts)
	namespaces := sortedSetKeys(nsSet)
	relations := sortedKeys(relationCounts)

	enrichedNodes := make([]enrichedNode, 0, len(snap.Graph.Nodes))
	for _, n := range snap.Graph.Nodes {
		enrichedNodes = append(enrichedNodes, h.enrichNode(r.Context(), n))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"version":    snap.Version,
		"from":       snap.From,
		"to":         snap.To,
		"created_at": snap.CreatedAt,
		"graph": map[string]any{
			"timestamp": snap.Graph.Timestamp,
			"nodes":     enrichedNodes,
			"edges":     snap.Graph.Edges,
		},
		"summary": map[string]any{
			"node_count":           len(snap.Graph.Nodes),
			"edge_count":           len(snap.Graph.Edges),
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

	kindFilter := entities.EntityKind(r.URL.Query().Get("kind"))
	nsFilter := r.URL.Query().Get("namespace")

	var filtered []enrichedNode
	for _, n := range snap.Graph.Nodes {
		if kindFilter != "" && n.Kind != kindFilter {
			continue
		}
		en := h.enrichNode(r.Context(), n)
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

	var found *topology.Node
	for i := range snap.Graph.Nodes {
		if snap.Graph.Nodes[i].ID == nodeID {
			found = &snap.Graph.Nodes[i]
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	enriched := h.enrichNode(r.Context(), *found)

	outbound, err := h.graphStore.QueryEdges(r.Context(), query.EdgeQuery{Source: nodeID})
	if err != nil {
		h.logger.Warn("failed to query outbound edges", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query edges")
		return
	}
	inbound, err := h.graphStore.QueryEdges(r.Context(), query.EdgeQuery{Target: nodeID})
	if err != nil {
		h.logger.Warn("failed to query inbound edges", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query edges")
		return
	}

	outboundEnriched := make([]enrichedEdge, 0, len(outbound))
	for _, e := range outbound {
		related := h.resolveEntitySummary(r.Context(), e.Target)
		outboundEnriched = append(outboundEnriched, toEnrichedEdge(e, related))
	}
	inboundEnriched := make([]enrichedEdge, 0, len(inbound))
	for _, e := range inbound {
		related := h.resolveEntitySummary(r.Context(), e.Source)
		inboundEnriched = append(inboundEnriched, toEnrichedEdge(e, related))
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

	edges, err := h.graphStore.QueryEdges(r.Context(), query.EdgeQuery{
		Source: sourceFilter,
		Target: targetFilter,
	})
	if err != nil {
		h.logger.Warn("failed to query edges", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to query edges")
		return
	}

	if relationFilter != "" {
		var filtered []topology.Edge
		for _, e := range edges {
			if e.Relation == relationFilter {
				filtered = append(filtered, e)
			}
		}
		edges = filtered
	}

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
