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
	require.Len(t, edges, 1)
	require.Equal(t, topology.RelExposes, edges[0].Relation)
	require.Equal(t, prof.Services[0].ID(), edges[0].Source)
	require.Equal(t, prof.Workloads[0].ID(), edges[0].Target)
	require.Equal(t, []string{"k8s-api"}, edges[0].Provenance)
	require.Len(t, nodes, 2) // service + workload
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
	require.Empty(t, edges)
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
	require.Empty(t, edges)
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
	require.Len(t, edges, 2)

	relMap := make(map[topology.RelationType]int)
	for _, e := range edges {
		relMap[e.Relation]++
		require.Equal(t, topology.RelUses, e.Relation)
		require.Equal(t, w.ID(), e.Source)
	}
	require.Equal(t, 2, relMap[topology.RelUses])
	require.Len(t, nodes, 3) // workload + configmap + secret
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
	require.Len(t, edges, 1)
	require.Equal(t, topology.RelMounts, edges[0].Relation)
	require.Equal(t, w.ID(), edges[0].Source)
	require.Len(t, nodes, 2) // workload + pvc
}

func TestStructuralBuilder_EmptyProfileProducesNothing(t *testing.T) {
	prof := &profile.ClusterProfile{ClusterName: "test"}

	builder := NewK8sStructuralTopologyBuilder("test")
	edges, nodes, err := builder.BuildStructural(context.Background(), prof)
	require.NoError(t, err)
	require.Empty(t, edges)
	require.Empty(t, nodes)
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
	require.Empty(t, edges)
}

func workloadWithPodLabels(cluster, ns, name string, podLabels map[string]string) entities.Workload {
	w := entities.NewWorkload(cluster, ns, name, entities.WorkloadDeployment, nil)
	w.PodTemplateLabels = podLabels
	return w
}
