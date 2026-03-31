package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/erickivel/oksg/pkg/core/entities"
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

	labels := entity.Labels()
	cloned := make(map[string]string, len(labels))
	for k, v := range labels {
		cloned[k] = v
	}

	r.entities[entity.ID()] = StoredEntity{
		EntityID:   entity.ID(),
		EntityKind: entity.Kind(),
		Cluster:    labels["cluster"],
		Namespace:  labels["namespace"],
		Name:       labels["name"],
		Metadata:   cloned,
	}
	return nil
}

// GetEntity returns the entity with the given ID.
// Returns an error if the entity is not found.
func (r *InMemoryEntityRegistry) GetEntity(_ context.Context, id entities.EntityID) (entities.Entity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entities[id]
	if !ok {
		return nil, fmt.Errorf("entity not found: %s", id)
	}
	return e, nil
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
		result = append(result, e)
	}
	return result, nil
}
