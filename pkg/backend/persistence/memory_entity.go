package persistence

import (
	"context"
	"fmt"
	"sync"

	"github.com/erickivel/oksg/pkg/core/entities"
)

// firstNonEmpty returns the first non-empty value found in m for the given keys.
func firstNonEmpty(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := m[k]; v != "" {
			return v
		}
	}
	return ""
}

// entityNameFromLabels resolves the display name from OTLP-style labels,
// falling back through kind-specific keys.
func entityNameFromLabels(kind entities.EntityKind, labels map[string]string) string {
	if v := labels["name"]; v != "" {
		return v
	}
	switch kind {
	case "container":
		return labels["k8s.container.name"]
	case "pod":
		return labels["k8s.pod.name"]
	case "namespace":
		return labels["k8s.namespace.name"]
	case "node":
		return labels["k8s.node.name"]
	case "service":
		return labels["service.name"]
	default:
		return ""
	}
}

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

	stored := StoredEntity{
		EntityID:   entity.ID(),
		EntityKind: entity.Kind(),
		Metadata:   cloned,
	}

	// If the caller passed a StoredEntity with fields already set, preserve them.
	if se, ok := entity.(StoredEntity); ok {
		stored.Cluster = se.Cluster
		stored.Namespace = se.Namespace
		stored.Name = se.Name
	}
	if sePt, ok := entity.(*StoredEntity); ok {
		stored.Cluster = sePt.Cluster
		stored.Namespace = sePt.Namespace
		stored.Name = sePt.Name
	}

	// Fall back to label-based derivation when fields are still empty.
	if stored.Cluster == "" {
		stored.Cluster = firstNonEmpty(labels, "cluster", "k8s.cluster.name")
	}
	if stored.Namespace == "" {
		stored.Namespace = firstNonEmpty(labels, "namespace", "k8s.namespace.name")
	}
	if stored.Name == "" {
		stored.Name = entityNameFromLabels(entity.Kind(), labels)
	}

	r.entities[entity.ID()] = stored
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
