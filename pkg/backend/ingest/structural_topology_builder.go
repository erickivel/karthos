package ingest

import (
	"context"
	"strconv"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const structuralProvenance = "k8s-api"

// K8sStructuralTopologyBuilder derives topology edges and nodes from Kubernetes
// structural data in a ClusterProfile. It produces exposes, routes_to, uses,
// and mounts edges that complement the runtime edges from metric and trace builders.
type K8sStructuralTopologyBuilder struct {
	clusterName string
}

func NewK8sStructuralTopologyBuilder(clusterName string) *K8sStructuralTopologyBuilder {
	return &K8sStructuralTopologyBuilder{clusterName: clusterName}
}

func (b *K8sStructuralTopologyBuilder) BuildStructural(_ context.Context, prof *profile.ClusterProfile) ([]topology.Edge, []topology.Node, error) {
	now := time.Now().UTC()
	edgeMap := make(map[string]topology.Edge)
	nodeMap := make(map[entities.EntityID]topology.Node)

	b.buildExposesEdges(prof, now, edgeMap, nodeMap)
	b.buildRoutesToEdges(prof, now, edgeMap, nodeMap)
	b.buildUsesAndMountsEdges(prof, now, edgeMap, nodeMap)
	b.buildWorkloadPodContainment(prof, now, edgeMap, nodeMap)

	edges := make([]topology.Edge, 0, len(edgeMap))
	for _, e := range edgeMap {
		edges = append(edges, e)
	}

	nodes := make([]topology.Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	return edges, nodes, nil
}

// buildExposesEdges matches Service selectors against Workload pod template labels.
func (b *K8sStructuralTopologyBuilder) buildExposesEdges(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	for _, svc := range prof.Services {
		if len(svc.Selector) == 0 {
			continue
		}
		for _, w := range prof.Workloads {
			if svc.Namespace != w.Namespace {
				continue
			}
			if !selectorMatches(svc.Selector, w.PodTemplateLabels) {
				continue
			}
			e := topology.NewEdge(svc.ID(), w.ID(), topology.RelExposes, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, newNode(
				svc.ID(),
				entities.KindService,
				svc.ClusterName,
				svc.Namespace,
				svc.Name,
				svc.Labels(),
			))
			addNode(nodes, workloadNode(w))
		}
	}
}

// buildWorkloadPodContainment connects discovered workload controllers to their
// pods so runtime/detail projections can expand traffic participants to pods.
func (b *K8sStructuralTopologyBuilder) buildWorkloadPodContainment(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	workloadByID := make(map[entities.EntityID]entities.Workload)
	for _, w := range prof.Workloads {
		workloadByID[w.ID()] = w
	}

	for _, p := range prof.Pods {
		if p.WorkloadID == "" {
			continue
		}
		w, ok := workloadByID[p.WorkloadID]
		if !ok {
			continue
		}
		e := topology.NewEdge(w.ID(), p.ID(), topology.RelContains, t)
		e.Provenance = []string{structuralProvenance}
		edges[e.ID] = e
		addNode(nodes, workloadNode(w))
		addNode(nodes, newNode(
			p.ID(),
			entities.KindPod,
			p.ClusterName,
			p.Namespace,
			p.Name,
			p.Labels(),
		))
	}
}

// buildRoutesToEdges connects Ingresses to their backend Services.
func (b *K8sStructuralTopologyBuilder) buildRoutesToEdges(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	svcByNSName := make(map[string]entities.EntityID)
	svcByID := make(map[entities.EntityID]entities.Service)
	for _, svc := range prof.Services {
		svcByNSName[svc.Namespace+"/"+svc.Name] = svc.ID()
		svcByID[svc.ID()] = svc
	}

	for _, ing := range prof.Ingresses {
		for _, backend := range ing.BackendServices {
			svcID, ok := svcByNSName[ing.Namespace+"/"+backend.ServiceName]
			if !ok {
				continue
			}
			svc := svcByID[svcID]
			e := topology.NewEdge(ing.ID(), svcID, topology.RelRoutesTo, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, newNode(
				ing.ID(),
				entities.KindIngress,
				ing.ClusterName,
				ing.Namespace,
				ing.Name,
				ing.Labels(),
			))
			addNode(nodes, newNode(
				svcID,
				entities.KindService,
				svc.ClusterName,
				svc.Namespace,
				svc.Name,
				svc.Labels(),
			))
		}
	}
}

// buildUsesAndMountsEdges connects Workloads to ConfigMaps, Secrets, and PVCs.
func (b *K8sStructuralTopologyBuilder) buildUsesAndMountsEdges(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	for _, w := range prof.Workloads {
		wID := w.ID()
		addNode(nodes, workloadNode(w))

		for _, ref := range w.ConfigMapRefs {
			cmID := entities.ComputeEntityID(entities.KindConfigMap, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, cmID, topology.RelUses, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, newNode(
				cmID,
				entities.KindConfigMap,
				b.clusterName,
				w.Namespace,
				ref,
				nil,
			))
		}

		for _, ref := range w.SecretRefs {
			sID := entities.ComputeEntityID(entities.KindSecret, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, sID, topology.RelUses, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, newNode(
				sID,
				entities.KindSecret,
				b.clusterName,
				w.Namespace,
				ref,
				nil,
			))
		}

		for _, ref := range w.PVCRefs {
			pID := entities.ComputeEntityID(entities.KindPVC, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, pID, topology.RelMounts, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, newNode(
				pID,
				entities.KindPVC,
				b.clusterName,
				w.Namespace,
				ref,
				nil,
			))
		}
	}
}

func selectorMatches(selector, podTemplateLabels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if podTemplateLabels[k] != v {
			return false
		}
	}
	return true
}

func newNode(
	id entities.EntityID,
	kind entities.EntityKind,
	cluster string,
	namespace string,
	name string,
	labels map[string]string,
) topology.Node {
	return topology.Node{
		ID:     id,
		Kind:   kind,
		Labels: buildEntityMetadata(cluster, namespace, name, labels),
	}
}

func addNode(nodes map[entities.EntityID]topology.Node, node topology.Node) {
	if _, ok := nodes[node.ID]; !ok {
		nodes[node.ID] = node
	}
}

func workloadNode(w entities.Workload) topology.Node {
	return newNode(
		w.ID(),
		entities.KindWorkload,
		w.ClusterName,
		w.Namespace,
		w.Name,
		workloadLabels(w),
	)
}

func workloadLabels(w entities.Workload) map[string]string {
	labels := w.Labels()
	if labels == nil {
		labels = make(map[string]string)
	}
	if w.Replicas > 0 {
		labels["k8s.workload.replicas"] = strconv.FormatInt(int64(w.Replicas), 10)
	}
	return labels
}
