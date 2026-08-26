package persistence

import "github.com/erickivel/oksg/pkg/core/entities"

// MaterializeEntity converts any canonical entity into the one storage record
// shared by memory and PostgreSQL registries. Explicit StoredEntity fields win;
// label fallbacks support generic internal writers.
func MaterializeEntity(entity entities.Entity) StoredEntity {
	labels := cloneStringMap(entity.Labels())
	stored := StoredEntity{EntityID: entity.ID(), EntityKind: entity.Kind(), Metadata: labels}
	if source, ok := entity.(StoredEntity); ok {
		stored.Cluster, stored.Namespace, stored.Name = source.Cluster, source.Namespace, source.Name
	}
	if source, ok := entity.(*StoredEntity); ok {
		stored.Cluster, stored.Namespace, stored.Name = source.Cluster, source.Namespace, source.Name
	}
	if stored.Cluster == "" {
		stored.Cluster = firstNonEmpty(labels, "cluster", "k8s.cluster.name")
	}
	if stored.Namespace == "" {
		stored.Namespace = firstNonEmpty(labels, "namespace", "k8s.namespace.name")
	}
	if stored.Name == "" {
		stored.Name = entityNameFromLabels(entity.Kind(), labels)
	}
	return stored
}

func firstNonEmpty(labels map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := labels[key]; value != "" {
			return value
		}
	}
	return ""
}

func entityNameFromLabels(kind entities.EntityKind, labels map[string]string) string {
	if name := labels["name"]; name != "" {
		return name
	}
	switch kind {
	case entities.KindContainer:
		return labels["k8s.container.name"]
	case entities.KindPod:
		return labels["k8s.pod.name"]
	case entities.KindNamespace:
		return labels["k8s.namespace.name"]
	case entities.KindNode:
		return labels["k8s.node.name"]
	case entities.KindService:
		return labels["service.name"]
	default:
		return ""
	}
}
