package ingest

import (
	"context"
	"reflect"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
)

// envelopeResolverFactory is optional: custom resolvers keep their existing
// behavior. A scope belongs to one sequential Normalize call, not the shared
// normalizer, so concurrent envelopes cannot suppress each other's heartbeat.
type envelopeResolverFactory interface {
	ForEnvelope() EntityResolver
}

// ForEnvelope returns a resolver that persists the first occurrence of each
// entity metadata state in this envelope. Consecutive identical states refresh
// the registry heartbeat once; changes (including A -> B -> A) persist in order.
// Source-specific service and cluster resolution still use OTLPEntityResolver.
func (r *OTLPEntityResolver) ForEnvelope() EntityResolver {
	scoped := &OTLPEntityResolver{clusterName: r.clusterName}
	registry := r.registry
	if previous, ok := registry.(*envelopeEntityRegistry); ok {
		registry = previous.EntityRegistry
	}
	if registry != nil {
		scoped.registry = &envelopeEntityRegistry{EntityRegistry: registry, last: make(map[entities.EntityID]persistence.StoredEntity)}
	}
	return scoped
}

// Read methods deliberately forward to the live registry. In particular,
// namespace-less service disambiguation must observe newly registered services.
type envelopeEntityRegistry struct {
	persistence.EntityRegistry
	last map[entities.EntityID]persistence.StoredEntity
}

func (r *envelopeEntityRegistry) UpsertEntity(ctx context.Context, entity entities.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stored := persistence.MaterializeEntity(entity)
	if previous, ok := r.last[stored.EntityID]; ok && reflect.DeepEqual(previous, stored) {
		return nil
	}
	if err := r.EntityRegistry.UpsertEntity(ctx, stored); err != nil {
		return err
	}
	// Materialize owns the labels independently of mutable source maps and the
	// registry's copy. Failed writes never populate this request-local record.
	r.last[stored.EntityID] = persistence.MaterializeEntity(stored)
	return nil
}
