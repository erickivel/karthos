package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/erickivel/oksg/pkg/core/entities"
)

// ErrUnresolvableEntity is returned by EntityResolver when no known OTLP semantic convention
// attributes are present in the resource map. Callers may use errors.Is or errors.As to detect
// this condition and treat it as a skippable warning rather than a hard failure.
var ErrUnresolvableEntity = errors.New("cannot resolve entity: no known attributes in resource map")

// OTLPEntityResolver resolves OTLP semantic convention resource attributes to canonical EntityIDs.
type OTLPEntityResolver struct {
	clusterName string
}

func NewOTLPEntityResolver(clusterName string) *OTLPEntityResolver {
	return &OTLPEntityResolver{clusterName: clusterName}
}

// Resolve maps OTLP resource attributes to a canonical EntityID using OTLP semantic conventions.
// Resolution is attempted in priority order: container, pod, node, service.
// Returns ErrUnresolvableEntity when no known attributes are present.
func (r *OTLPEntityResolver) Resolve(_ context.Context, resource map[string]string) (entities.EntityID, error) {
	containerName, hasContainer := resource["k8s.container.name"]
	podName, hasPod := resource["k8s.pod.name"]
	namespace, hasNamespace := resource["k8s.namespace.name"]
	nodeName, hasNode := resource["k8s.node.name"]
	serviceName, hasService := resource["service.name"]

	// 1. Container: requires container name, pod name, and namespace
	if hasContainer && hasPod && hasNamespace {
		return entities.ComputeEntityID(entities.KindContainer, r.clusterName, namespace, podName, containerName), nil
	}

	// 2. Pod: requires pod name and namespace
	if hasPod && hasNamespace {
		return entities.ComputeEntityID(entities.KindPod, r.clusterName, namespace, podName), nil
	}

	// 3. Node: requires node name
	if hasNode {
		return entities.ComputeEntityID(entities.KindNode, r.clusterName, nodeName), nil
	}

	// 4. Service: requires service name; namespace is optional (best-effort)
	if hasService {
		if hasNamespace {
			return entities.ComputeEntityID(entities.KindService, r.clusterName, namespace, serviceName), nil
		}
		return entities.ComputeEntityID(entities.KindService, r.clusterName, serviceName), nil
	}

	return "", fmt.Errorf("resolve entity: %w", ErrUnresolvableEntity)
}
