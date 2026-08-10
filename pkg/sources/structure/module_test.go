package structure

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func TestModuleRefreshesLiveKubernetesTopology(t *testing.T) {
	client := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{
						Name: "api",
						EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"},
						}}},
					}}},
				},
			},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "demo", Labels: map[string]string{"app": "api"}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "api"}}}}}}}}},
		},
	)
	graphStore := persistence.NewInMemoryGraphStore()
	entitiesStore := persistence.NewInMemoryEntityRegistry()

	module := New(client, "test", graphStore, entitiesStore)
	require.NoError(t, module.Refresh(context.Background()))

	snapshot, err := graphStore.GetLatest(context.Background())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelExposes), 1)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelRoutesTo), 1)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelContains), 2)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelUses), 1)

	_, err = entitiesStore.GetEntity(context.Background(), entities.NewWorkload("test", "demo", "api", entities.WorkloadDeployment, nil).ID())
	require.NoError(t, err)
	_, err = entitiesStore.GetEntity(context.Background(), entities.ComputeEntityID(entities.KindConfigMap, "test", "demo", "api-config"))
	require.NoError(t, err)
}

func edgesByRelation(edges []topology.Edge, relation topology.RelationType) []topology.Edge {
	matched := make([]topology.Edge, 0)
	for _, edge := range edges {
		if edge.Relation == relation {
			matched = append(matched, edge)
		}
	}
	return matched
}
