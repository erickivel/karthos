package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
)

// ErrUnresolvableEntity is returned by EntityResolver when no known OTLP semantic convention
// attributes are present in the resource map. Callers may use errors.Is or errors.As to detect
// this condition and treat it as a skippable warning rather than a hard failure.
var ErrUnresolvableEntity = errors.New("cannot resolve entity: no known attributes in resource map")

// OTLPEntityResolver resolves OTLP semantic convention resource attributes to canonical EntityIDs.
// When an EntityRegistry is provided, resolved entities are upserted on each resolution.
type OTLPEntityResolver struct {
	clusterName string
	registry    persistence.EntityRegistry
}

func NewOTLPEntityResolver(clusterName string) *OTLPEntityResolver {
	return &OTLPEntityResolver{clusterName: clusterName}
}

// NewOTLPEntityResolverWithRegistry creates a resolver that persists entities to the given registry.
func NewOTLPEntityResolverWithRegistry(clusterName string, registry persistence.EntityRegistry) *OTLPEntityResolver {
	return &OTLPEntityResolver{clusterName: clusterName, registry: registry}
}

// Resolve maps OTLP resource attributes to a canonical EntityID using OTLP semantic conventions.
// Resolution is attempted in priority order: container, pod, node, service.
// Returns ErrUnresolvableEntity when no known attributes are present.
func (r *OTLPEntityResolver) Resolve(ctx context.Context, resource map[string]string) (entities.EntityID, error) {
	containerName, hasContainer := resource["k8s.container.name"]
	podName, hasPod := resource["k8s.pod.name"]
	namespace, hasNamespace := resource["k8s.namespace.name"]
	nodeName, hasNode := resource["k8s.node.name"]
	serviceName, hasService := resource["service.name"]

	var id entities.EntityID
	var kind entities.EntityKind
	var name string

	switch {
	// 1. Container: requires container name, pod name, and namespace
	case hasContainer && hasPod && hasNamespace:
		id = entities.ComputeEntityID(entities.KindContainer, r.clusterName, namespace, podName, containerName)
		kind = entities.KindContainer
		name = containerName

	// 2. Pod: requires pod name and namespace
	case hasPod && hasNamespace:
		id = entities.ComputeEntityID(entities.KindPod, r.clusterName, namespace, podName)
		kind = entities.KindPod
		name = podName

	// 3. Node: requires node name
	case hasNode:
		id = entities.ComputeEntityID(entities.KindNode, r.clusterName, nodeName)
		kind = entities.KindNode
		name = nodeName

	// 4. Service: requires service name; namespace is optional (best-effort)
	case hasService && hasNamespace:
		id = entities.ComputeEntityID(entities.KindService, r.clusterName, namespace, serviceName)
		kind = entities.KindService
		name = serviceName

	case hasService:
		id = entities.ComputeEntityID(entities.KindService, r.clusterName, serviceName)
		kind = entities.KindService
		name = serviceName

	default:
		return "", fmt.Errorf("resolve entity: %w", ErrUnresolvableEntity)
	}

	if r.registry != nil {
		_ = r.registry.UpsertEntity(ctx, persistence.StoredEntity{
			EntityID:   id,
			EntityKind: kind,
			Cluster:    r.clusterName,
			Namespace:  namespace,
			Name:       name,
			Metadata:   resource,
		})
	}

	return id, nil
}
