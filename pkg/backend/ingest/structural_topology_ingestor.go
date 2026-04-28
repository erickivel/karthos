package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

// StructuralTopologyIngestor ingests K8s structural topology from a ClusterProfile
// into the graph store.
type StructuralTopologyIngestor struct {
	builder  StructuralTopologyBuilder
	store    persistence.GraphStore
	registry persistence.EntityRegistry
	logger   *logging.Logger
}

func NewStructuralTopologyIngestor(
	builder StructuralTopologyBuilder,
	store persistence.GraphStore,
	registry persistence.EntityRegistry,
	logger *logging.Logger,
) *StructuralTopologyIngestor {
	return &StructuralTopologyIngestor{builder: builder, store: store, registry: registry, logger: logger}
}

func (i *StructuralTopologyIngestor) IngestProfile(ctx context.Context, prof *profile.ClusterProfile) error {
	if i.registry != nil {
		if err := i.upsertStructuralEntities(ctx, prof); err != nil {
			return fmt.Errorf("upsert structural entities: %w", err)
		}
	} else if i.logger != nil {
		i.logger.Warn("structural ingest: entity registry not configured; discovered entities will not be upserted")
	}

	edges, nodes, err := i.builder.BuildStructural(ctx, prof)
	if err != nil {
		return fmt.Errorf("build structural topology: %w", err)
	}
	if len(edges) == 0 && len(nodes) == 0 {
		return nil
	}

	now := time.Now().UTC()
	snapshot := topology.Snapshot{
		Version: schema.CurrentVersion,
		Graph: topology.Graph{
			Nodes:     nodes,
			Edges:     edges,
			Timestamp: now,
		},
		From:      prof.DiscoveredAt,
		To:        now,
		CreatedAt: now,
	}
	return i.store.PutSnapshot(ctx, snapshot)
}

func (i *StructuralTopologyIngestor) upsertStructuralEntities(ctx context.Context, prof *profile.ClusterProfile) error {
	upsert := func(entity persistence.StoredEntity) error {
		return i.registry.UpsertEntity(ctx, entity)
	}
	upsertRefs := func(kind entities.EntityKind, cluster, namespace string, refs []string) error {
		for _, ref := range refs {
			if err := upsert(newStoredEntity(
				entities.ComputeEntityID(kind, cluster, namespace, ref),
				kind,
				cluster,
				namespace,
				ref,
				nil,
			)); err != nil {
				return err
			}
		}
		return nil
	}

	for _, svc := range prof.Services {
		if err := upsert(newStoredEntity(
			svc.ID(),
			entities.KindService,
			svc.ClusterName,
			svc.Namespace,
			svc.Name,
			svc.Labels(),
		)); err != nil {
			return err
		}
	}

	for _, workload := range prof.Workloads {
		if err := upsert(newStoredEntity(
			workload.ID(),
			entities.KindWorkload,
			workload.ClusterName,
			workload.Namespace,
			workload.Name,
			workloadLabels(workload),
		)); err != nil {
			return err
		}
		if err := upsertRefs(entities.KindConfigMap, prof.ClusterName, workload.Namespace, workload.ConfigMapRefs); err != nil {
			return err
		}
		if err := upsertRefs(entities.KindSecret, prof.ClusterName, workload.Namespace, workload.SecretRefs); err != nil {
			return err
		}
		if err := upsertRefs(entities.KindPVC, prof.ClusterName, workload.Namespace, workload.PVCRefs); err != nil {
			return err
		}
	}

	for _, pod := range prof.Pods {
		if err := upsert(newStoredEntity(
			pod.ID(),
			entities.KindPod,
			pod.ClusterName,
			pod.Namespace,
			pod.Name,
			pod.Labels(),
		)); err != nil {
			return err
		}
	}

	for _, ingress := range prof.Ingresses {
		if err := upsert(newStoredEntity(
			ingress.ID(),
			entities.KindIngress,
			ingress.ClusterName,
			ingress.Namespace,
			ingress.Name,
			ingress.Labels(),
		)); err != nil {
			return err
		}
	}

	return nil
}

func newStoredEntity(
	id entities.EntityID,
	kind entities.EntityKind,
	cluster string,
	namespace string,
	name string,
	labels map[string]string,
) persistence.StoredEntity {
	return persistence.StoredEntity{
		EntityID:   id,
		EntityKind: kind,
		Cluster:    cluster,
		Namespace:  namespace,
		Name:       name,
		Metadata:   buildEntityMetadata(cluster, namespace, name, labels),
	}
}
