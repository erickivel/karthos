package ingest

import (
	"context"
	"errors"
	"fmt"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/semconv"
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
// Resolution is attempted in priority order: container, pod, node, deployment,
// statefulset, replicaset, daemonset, job, cronjob, replicationcontroller,
// hpa, resourcequota, namespace, service.
// Returns ErrUnresolvableEntity when no known attributes are present.
func (r *OTLPEntityResolver) Resolve(ctx context.Context, resource map[string]string) (entities.EntityID, error) {
	containerName, hasContainer := resource[semconv.ResourceK8sContainerName]
	podName, hasPod := resource[semconv.ResourceK8sPodName]
	namespace, hasNamespace := resource[semconv.ResourceK8sNamespaceName]
	nodeName, hasNode := resource[semconv.ResourceK8sNodeName]
	serviceName, hasService := resource[semconv.ResourceServiceName]
	deploymentName, hasDeployment := resource[semconv.ResourceK8sDeploymentName]
	statefulSetName, hasStatefulSet := resource[semconv.ResourceK8sStatefulSetName]
	replicaSetName, hasReplicaSet := resource[semconv.ResourceK8sReplicaSetName]
	daemonSetName, hasDaemonSet := resource[semconv.ResourceK8sDaemonSetName]
	jobName, hasJob := resource[semconv.ResourceK8sJobName]
	cronJobName, hasCronJob := resource[semconv.ResourceK8sCronJobName]
	replicationControllerName, hasReplicationController := resource[semconv.ResourceK8sReplicationControllerName]
	hpaName, hasHPA := resource[semconv.ResourceK8sHPAName]
	resourceQuotaName, hasResourceQuota := resource[semconv.ResourceK8sResourceQuotaName]

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

	// 4. Deployment: requires deployment name and namespace
	case hasDeployment && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadDeployment), deploymentName)
		kind = entities.KindWorkload
		name = deploymentName

	// 5. StatefulSet: requires statefulset name and namespace
	case hasStatefulSet && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadStatefulSet), statefulSetName)
		kind = entities.KindWorkload
		name = statefulSetName

	// 6. ReplicaSet: requires replicaset name and namespace (no deployment present)
	case hasReplicaSet && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, "replicaset", replicaSetName)
		kind = entities.KindWorkload
		name = replicaSetName

	// 7. DaemonSet: requires daemonset name and namespace
	case hasDaemonSet && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadDaemonSet), daemonSetName)
		kind = entities.KindWorkload
		name = daemonSetName

	// 8. Job: requires job name and namespace
	case hasJob && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadJob), jobName)
		kind = entities.KindWorkload
		name = jobName

	// 9. CronJob: requires cronjob name and namespace
	case hasCronJob && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadCronJob), cronJobName)
		kind = entities.KindWorkload
		name = cronJobName

	// 10. ReplicationController: requires replicationcontroller name and namespace
	case hasReplicationController && hasNamespace:
		id = entities.ComputeEntityID(entities.KindWorkload, r.clusterName, namespace, string(entities.WorkloadReplicationController), replicationControllerName)
		kind = entities.KindWorkload
		name = replicationControllerName

	// 11. HPA: requires hpa name and namespace
	case hasHPA && hasNamespace:
		id = entities.ComputeEntityID(entities.KindHPA, r.clusterName, namespace, hpaName)
		kind = entities.KindHPA
		name = hpaName

	// 12. ResourceQuota: requires resourcequota name and namespace
	case hasResourceQuota && hasNamespace:
		id = entities.ComputeEntityID(entities.KindResourceQuota, r.clusterName, namespace, resourceQuotaName)
		kind = entities.KindResourceQuota
		name = resourceQuotaName

	// 13. Namespace: requires namespace name but no more specific entity or service
	case hasNamespace && !hasService:
		id = entities.ComputeEntityID(entities.KindNamespace, r.clusterName, namespace)
		kind = entities.KindNamespace
		name = namespace

	// 14. Service: requires service name; namespace is optional (best-effort)
	case hasService && hasNamespace:
		id = entities.ComputeEntityID(entities.KindService, r.clusterName, namespace, serviceName)
		kind = entities.KindService
		name = serviceName

	case hasService:
		if canonicalID, canonicalNS, ok := r.resolveCanonicalService(ctx, serviceName, ""); ok {
			id = canonicalID
			namespace = canonicalNS
		} else {
			id = entities.ComputeEntityID(entities.KindService, r.clusterName, serviceName)
		}
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

func (r *OTLPEntityResolver) resolveCanonicalService(ctx context.Context, serviceName, namespaceHint string) (entities.EntityID, string, bool) {
	if serviceName == "" {
		return "", "", false
	}
	if namespaceHint != "" {
		return entities.ComputeEntityID(entities.KindService, r.clusterName, namespaceHint, serviceName), namespaceHint, true
	}
	if r.registry == nil {
		return "", "", false
	}

	candidates, err := r.registry.ListEntities(ctx, persistence.EntityQuery{Kind: entities.KindService})
	if err != nil {
		return "", "", false
	}

	var namespaced []entities.Entity
	var fallback []entities.Entity
	for _, candidate := range candidates {
		candidateName := serviceEntityName(candidate)
		if candidateName != serviceName {
			continue
		}
		if candidateNS := serviceEntityNamespace(candidate); candidateNS != "" {
			namespaced = append(namespaced, candidate)
			continue
		}
		fallback = append(fallback, candidate)
	}

	if len(namespaced) == 1 {
		candidate := namespaced[0]
		return candidate.ID(), serviceEntityNamespace(candidate), true
	}
	if len(namespaced) == 0 && len(fallback) == 1 {
		return fallback[0].ID(), "", true
	}
	return "", "", false
}

func serviceEntityName(entity entities.Entity) string {
	if stored, ok := entity.(persistence.StoredEntity); ok && stored.Name != "" {
		return stored.Name
	}
	if stored, ok := entity.(*persistence.StoredEntity); ok && stored.Name != "" {
		return stored.Name
	}
	labels := entity.Labels()
	if labels[semconv.ResourceServiceName] != "" {
		return labels[semconv.ResourceServiceName]
	}
	return labels["name"]
}

func serviceEntityNamespace(entity entities.Entity) string {
	if stored, ok := entity.(persistence.StoredEntity); ok && stored.Namespace != "" {
		return stored.Namespace
	}
	if stored, ok := entity.(*persistence.StoredEntity); ok && stored.Namespace != "" {
		return stored.Namespace
	}
	labels := entity.Labels()
	if labels[semconv.ResourceK8sNamespaceName] != "" {
		return labels[semconv.ResourceK8sNamespaceName]
	}
	return labels["namespace"]
}
