package ingest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestStructuralBuilder_ServiceExposesWorkload(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}, nil),
		},
		Workloads: []entities.Workload{
			workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}),
		},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	exposes := findEdgesByRelation(edges, topology.RelExposes)
	require.Len(t, exposes, 1)
	require.Equal(t, prof.Services[0].ID(), exposes[0].Source)
	require.Equal(t, prof.Workloads[0].ID(), exposes[0].Target)
	require.Equal(t, []string{"k8s-api"}, exposes[0].Provenance)
	require.Len(t, nodes, 3) // namespace + service + workload
}

func TestStructuralBuilder_SelectorMismatchProducesNoEdge(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "demo", "gateway-go", map[string]string{"app": "other"}, nil),
		},
		Workloads: []entities.Workload{
			workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}),
		},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, _, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, findEdgesByRelation(edges, topology.RelExposes))
}

func TestStructuralBuilder_EmptySelectorProducesNoEdge(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "demo", "gateway-go", nil, nil),
		},
		Workloads: []entities.Workload{
			workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"}),
		},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, _, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, findEdgesByRelation(edges, topology.RelExposes))
}

func TestStructuralBuilder_IngressRoutesToService(t *testing.T) {
	svc := entities.NewService("test", "demo", "gateway-go", nil, nil)
	ing := entities.NewIngress("test", "demo", "gw-ingress", "", nil)
	ing.BackendServices = []entities.IngressBackend{{ServiceName: "gateway-go", ServicePort: 8080}}

	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services:    []entities.Service{svc},
		Ingresses:   []entities.Ingress{ing},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, topology.RelRoutesTo, edges[0].Relation)
	require.Equal(t, ing.ID(), edges[0].Source)
	require.Equal(t, svc.ID(), edges[0].Target)
	require.Len(t, nodes, 2) // ingress + service
}

func TestStructuralBuilder_IngressWithUnknownServiceProducesNoEdge(t *testing.T) {
	ing := entities.NewIngress("test", "demo", "gw-ingress", "", nil)
	ing.BackendServices = []entities.IngressBackend{{ServiceName: "nonexistent", ServicePort: 8080}}

	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Ingresses:   []entities.Ingress{ing},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, _, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, edges)
}

func TestStructuralBuilder_WorkloadUsesConfigMapAndSecret(t *testing.T) {
	w := workloadWithPodLabels("test", "demo", "gateway-go", nil)
	w.ConfigMapRefs = []string{"gateway-config"}
	w.SecretRefs = []string{"gateway-secret"}

	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Workloads:   []entities.Workload{w},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Len(t, findEdgesByRelation(edges, topology.RelUses), 2)

	relMap := make(map[topology.RelationType]int)
	for _, e := range edges {
		relMap[e.Relation]++
		if e.Relation != topology.RelUses {
			continue
		}
		require.Equal(t, topology.RelUses, e.Relation)
		require.Equal(t, w.ID(), e.Source)
	}
	require.Equal(t, 2, relMap[topology.RelUses])
	require.Equal(t, 1, relMap[topology.RelContains])
	require.Len(t, nodes, 4) // namespace + workload + configmap + secret
}

func TestStructuralBuilder_WorkloadMountsPVC(t *testing.T) {
	w := workloadWithPodLabels("test", "demo", "db", nil)
	w.PVCRefs = []string{"data-volume"}

	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Workloads:   []entities.Workload{w},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	mounts := findEdgesByRelation(edges, topology.RelMounts)
	require.Len(t, mounts, 1)
	require.Equal(t, w.ID(), mounts[0].Source)
	require.Len(t, nodes, 3) // namespace + workload + pvc
}

func TestStructuralBuilder_WorkloadContainsMatchingPods(t *testing.T) {
	w := workloadWithPodLabels("test", "demo", "gateway-go", map[string]string{"app": "gateway-go"})
	pod := entities.NewPod("test", "demo", "gateway-go-abc", map[string]string{"app": "gateway-go"})
	pod.WorkloadID = w.ID()

	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Workloads:   []entities.Workload{w},
		Pods:        []entities.Pod{pod},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	contains := findEdgesByRelation(edges, topology.RelContains)
	require.Len(t, contains, 2)
	require.True(t, hasEdge(contains, w.ID(), pod.ID(), topology.RelContains))
	require.Len(t, nodes, 3)
}

func TestStructuralBuilder_EmptyProfileProducesNothing(t *testing.T) {
	prof := &profile.ClusterProfile{ClusterName: "test"}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, edges)
	require.Empty(t, nodes)
}

func TestStructuralBuilder_MaterializesStandaloneMetricEntities(t *testing.T) {
	workload := entities.NewWorkload("test", "demo", "nightly", entities.WorkloadCronJob, nil)
	hpa := entities.NewHPA("test", "demo", "gateway-hpa", nil)
	quota := entities.NewResourceQuota("test", "demo", "compute", nil)
	prof := &profile.ClusterProfile{
		ClusterName:    "test",
		Workloads:      []entities.Workload{workload},
		HPAs:           []entities.HPA{hpa},
		ResourceQuotas: []entities.ResourceQuota{quota},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)

	nodeByID := map[entities.EntityID]topology.Node{}
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	require.Equal(t, entities.KindWorkload, nodeByID[workload.ID()].Kind)
	require.Equal(t, entities.KindHPA, nodeByID[hpa.ID()].Kind)
	require.Equal(t, entities.KindResourceQuota, nodeByID[quota.ID()].Kind)
	require.Equal(t, string(entities.WorkloadCronJob), nodeByID[workload.ID()].Labels[entities.WorkloadTypeLabel])

	namespaceID := entities.ComputeEntityID(entities.KindNamespace, "test", "demo")
	contains := findEdgesByRelation(edges, topology.RelContains)
	require.True(t, hasEdge(contains, namespaceID, workload.ID(), topology.RelContains))
	require.True(t, hasEdge(contains, namespaceID, hpa.ID(), topology.RelContains))
	require.True(t, hasEdge(contains, namespaceID, quota.ID(), topology.RelContains))
}

func TestStructuralBuilder_CrossNamespaceServiceDoesNotMatchWorkload(t *testing.T) {
	prof := &profile.ClusterProfile{
		ClusterName: "test",
		Services: []entities.Service{
			entities.NewService("test", "ns-a", "svc", map[string]string{"app": "x"}, nil),
		},
		Workloads: []entities.Workload{
			workloadWithPodLabels("test", "ns-b", "x", map[string]string{"app": "x"}),
		},
	}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, _, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, findEdgesByRelation(edges, topology.RelExposes))
}

func workloadWithPodLabels(cluster, ns, name string, podLabels map[string]string) entities.Workload {
	w := entities.NewWorkload(cluster, ns, name, entities.WorkloadDeployment, nil)
	w.PodTemplateLabels = podLabels
	return w
}

func findEdgesByRelation(edges []topology.Edge, relation topology.RelationType) []topology.Edge {
	var out []topology.Edge
	for _, edge := range edges {
		if edge.Relation == relation {
			out = append(out, edge)
		}
	}
	return out
}

func hasEdge(edges []topology.Edge, source, target entities.EntityID, relation topology.RelationType) bool {
	for _, edge := range edges {
		if edge.Source == source && edge.Target == target && edge.Relation == relation {
			return true
		}
	}
	return false
}
