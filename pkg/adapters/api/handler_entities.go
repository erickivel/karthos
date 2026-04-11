package api

import (
	"cmp"
	"errors"
	"net/http"
	"slices"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
)

// EntityHandler serves entity query endpoints.
type EntityHandler struct {
	entityRegistry persistence.EntityRegistry
	logger         *logging.Logger
}

// RegisterRoutes registers entity endpoints on the given mux.
func (h *EntityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/entities", h.handleListEntities)
	mux.HandleFunc("GET /api/v1/entities/{id}", h.handleGetEntity)
}

func (h *EntityHandler) handleListEntities(w http.ResponseWriter, r *http.Request) {
	if h.entityRegistry == nil {
		writeError(w, http.StatusNotFound, "entity registry not configured")
		return
	}

	q := persistence.EntityQuery{
		Kind:      entities.EntityKind(r.URL.Query().Get("kind")),
		Namespace: r.URL.Query().Get("namespace"),
	}

	result, err := h.entityRegistry.ListEntities(r.Context(), q)
	if err != nil {
		h.logger.Warn("failed to list entities", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list entities")
		return
	}
	if result == nil {
		result = []entities.Entity{}
	}

	slices.SortFunc(result, func(a, b entities.Entity) int {
		if c := cmp.Compare(string(a.Kind()), string(b.Kind())); c != 0 {
			return c
		}
		return cmp.Compare(string(a.ID()), string(b.ID()))
	})

	total := len(result)
	pag := parsePaginationParams(r)
	page, nextToken := applyPagination(result, pag)

	writeJSONPaginated(w, http.StatusOK, map[string]any{
		"entities": page,
		"total":    total,
	}, nextToken)
}

func (h *EntityHandler) handleGetEntity(w http.ResponseWriter, r *http.Request) {
	if h.entityRegistry == nil {
		writeError(w, http.StatusNotFound, "entity registry not configured")
		return
	}

	id := entities.EntityID(r.PathValue("id"))

	entity, err := h.entityRegistry.GetEntity(r.Context(), id)
	if err != nil {
		if errors.Is(err, persistence.ErrEntityNotFound) {
			writeError(w, http.StatusNotFound, "entity not found")
			return
		}
		h.logger.Warn("failed to get entity", logging.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve entity")
		return
	}

	writeJSON(w, http.StatusOK, entity)
}
