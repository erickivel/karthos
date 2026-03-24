package persistence

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/entities"
)

// EntityQuery defines filters for listing entities.
type EntityQuery struct {
	Kind      entities.EntityKind
	Namespace string
}

// EntityRegistry manages canonical entity records. Upsert is idempotent:
// writing an entity with the same ID updates its labels and last-seen time.
type EntityRegistry interface {
	UpsertEntity(ctx context.Context, entity entities.Entity) error
	GetEntity(ctx context.Context, id entities.EntityID) (entities.Entity, error)
	ListEntities(ctx context.Context, q EntityQuery) ([]entities.Entity, error)
}

// StoredEntity is a materialized Entity read back from persistence. It
// implements the Entity interface without coupling to a specific concrete
// entity struct (Pod, Service, etc.).
type StoredEntity struct {
	EntityID   entities.EntityID   `json:"id"`
	EntityKind entities.EntityKind `json:"kind"`
	Cluster    string              `json:"cluster"`
	Namespace  string              `json:"namespace,omitempty"`
	Name       string              `json:"name"`
	Metadata   map[string]string   `json:"labels,omitempty"`
}

// ID returns the stable entity identifier.
func (e StoredEntity) ID() entities.EntityID { return e.EntityID }

// Kind returns the entity kind.
func (e StoredEntity) Kind() entities.EntityKind { return e.EntityKind }

// Labels returns the entity metadata labels.
func (e StoredEntity) Labels() map[string]string { return e.Metadata }
