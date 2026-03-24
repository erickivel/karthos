package ingest

import (
	"context"
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
			addNode(nodes, svc.ID(), entities.KindService)
			addNode(nodes, w.ID(), entities.KindWorkload)
		}
	}
}

// buildRoutesToEdges connects Ingresses to their backend Services.
func (b *K8sStructuralTopologyBuilder) buildRoutesToEdges(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	svcByNSName := make(map[string]entities.EntityID)
	for _, svc := range prof.Services {
		svcByNSName[svc.Namespace+"/"+svc.Name] = svc.ID()
	}

	for _, ing := range prof.Ingresses {
		for _, backend := range ing.BackendServices {
			svcID, ok := svcByNSName[ing.Namespace+"/"+backend.ServiceName]
			if !ok {
				continue
			}
			e := topology.NewEdge(ing.ID(), svcID, topology.RelRoutesTo, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, ing.ID(), entities.KindIngress)
			addNode(nodes, svcID, entities.KindService)
		}
	}
}

// buildUsesAndMountsEdges connects Workloads to ConfigMaps, Secrets, and PVCs.
func (b *K8sStructuralTopologyBuilder) buildUsesAndMountsEdges(prof *profile.ClusterProfile, t time.Time, edges map[string]topology.Edge, nodes map[entities.EntityID]topology.Node) {
	for _, w := range prof.Workloads {
		wID := w.ID()
		addNode(nodes, wID, entities.KindWorkload)

		for _, ref := range w.ConfigMapRefs {
			cmID := entities.ComputeEntityID(entities.KindConfigMap, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, cmID, topology.RelUses, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, cmID, entities.KindConfigMap)
		}

		for _, ref := range w.SecretRefs {
			sID := entities.ComputeEntityID(entities.KindSecret, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, sID, topology.RelUses, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, sID, entities.KindSecret)
		}

		for _, ref := range w.PVCRefs {
			pID := entities.ComputeEntityID(entities.KindPVC, b.clusterName, w.Namespace, ref)
			e := topology.NewEdge(wID, pID, topology.RelMounts, t)
			e.Provenance = []string{structuralProvenance}
			edges[e.ID] = e
			addNode(nodes, pID, entities.KindPVC)
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

func addNode(nodes map[entities.EntityID]topology.Node, id entities.EntityID, kind entities.EntityKind) {
	if _, ok := nodes[id]; !ok {
		nodes[id] = topology.Node{ID: id, Kind: kind}
	}
}
