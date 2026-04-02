package api

import (
	"net/http"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// GraphHandler serves topology graph query endpoints.
type GraphHandler struct {
	graphStore persistence.GraphStore
	logger     *logging.Logger
}

// RegisterRoutes registers graph endpoints on the given mux.
func (h *GraphHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/graph", h.handleGetGraph)
	mux.HandleFunc("GET /api/v1/graph/nodes", h.handleListNodes)
	mux.HandleFunc("GET /api/v1/graph/nodes/{id}", h.handleGetNode)
	mux.HandleFunc("GET /api/v1/graph/edges", h.handleListEdges)
}

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

	writeJSON(w, http.StatusOK, map[string]any{
		"version":    snap.Version,
		"from":       snap.From,
		"to":         snap.To,
		"created_at": snap.CreatedAt,
		"graph":      snap.Graph,
		"summary": map[string]int{
			"node_count": len(snap.Graph.Nodes),
			"edge_count": len(snap.Graph.Edges),
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
		writeJSON(w, http.StatusOK, map[string]any{"nodes": []topology.Node{}, "total": 0})
		return
	}

	kindFilter := entities.EntityKind(r.URL.Query().Get("kind"))
	nsFilter := r.URL.Query().Get("namespace")

	var filtered []topology.Node
	for _, n := range snap.Graph.Nodes {
		if kindFilter != "" && n.Kind != kindFilter {
			continue
		}
		if nsFilter != "" && n.Labels["namespace"] != nsFilter {
			continue
		}
		filtered = append(filtered, n)
	}
	if filtered == nil {
		filtered = []topology.Node{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": filtered,
		"total": len(filtered),
	})
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
	if outbound == nil {
		outbound = []topology.Edge{}
	}
	if inbound == nil {
		inbound = []topology.Edge{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"node": found,
		"edges": map[string]any{
			"outbound": outbound,
			"inbound":  inbound,
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

	writeJSON(w, http.StatusOK, map[string]any{
		"edges": edges,
		"total": len(edges),
	})
}
