package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/erickivel/karthos/pkg/core/entities"
)

// InMemoryEntityRegistry is a thread-safe in-memory implementation of EntityRegistry.
type InMemoryEntityRegistry struct {
	mu       sync.RWMutex
	entities map[entities.EntityID]StoredEntity
}

// NewInMemoryEntityRegistry returns a new empty InMemoryEntityRegistry.
func NewInMemoryEntityRegistry() *InMemoryEntityRegistry {
	return &InMemoryEntityRegistry{
		entities: make(map[entities.EntityID]StoredEntity),
	}
}

// UpsertEntity stores the entity, replacing any previous entry with the same ID.
func (r *InMemoryEntityRegistry) UpsertEntity(_ context.Context, entity entities.Entity) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entities[entity.ID()] = MaterializeEntity(entity)
	return nil
}

// GetEntity returns the entity with the given ID.
// Returns an error if the entity is not found.
func (r *InMemoryEntityRegistry) GetEntity(_ context.Context, id entities.EntityID) (entities.Entity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entities[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrEntityNotFound, id)
	}
	e.Metadata = cloneStringMap(e.Metadata)
	return e, nil
}

// GetEntities returns the entities for the given IDs. Missing IDs are simply
// absent from the returned map; callers must treat absence as not-found.
func (r *InMemoryEntityRegistry) GetEntities(_ context.Context, ids []entities.EntityID) (map[entities.EntityID]entities.Entity, error) {
	out := make(map[entities.EntityID]entities.Entity, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range ids {
		if e, ok := r.entities[id]; ok {
			e.Metadata = cloneStringMap(e.Metadata)
			out[id] = e
		}
	}
	return out, nil
}

// ListEntities returns entities matching the query filters. Kind and Namespace
// are only applied when non-empty.
func (r *InMemoryEntityRegistry) ListEntities(_ context.Context, q EntityQuery) ([]entities.Entity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []entities.Entity
	for _, e := range r.entities {
		if q.Kind != "" && e.EntityKind != q.Kind {
			continue
		}
		if q.Namespace != "" && e.Namespace != q.Namespace {
			continue
		}
		e.Metadata = cloneStringMap(e.Metadata)
		result = append(result, e)
	}
	return result, nil
}
