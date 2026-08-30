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

func TestModuleIncludesDirectlyListedObjectsWithoutResolvedSemanticTargets(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "demo"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "external-db", Namespace: "demo"}},
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "demo"},
			Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{Paths: []networkingv1.HTTPIngressPath{{
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{Name: "missing-service"}},
				}}}},
			}}},
		},
	)
	graphStore := persistence.NewInMemoryGraphStore()
	module := New(client, "test", graphStore, persistence.NewInMemoryEntityRegistry())
	require.NoError(t, module.Refresh(context.Background()))

	snapshot, err := graphStore.GetLatest(context.Background())
	require.NoError(t, err)
	nodes := map[entities.EntityID]topology.Node{}
	for _, node := range snapshot.Graph.Nodes {
		nodes[node.ID] = node
	}
	for _, expected := range []entities.EntityID{
		entities.ComputeEntityID(entities.KindPod, "test", "demo", "standalone"),
		entities.ComputeEntityID(entities.KindService, "test", "demo", "external-db"),
		entities.ComputeEntityID(entities.KindIngress, "test", "demo", "public"),
	} {
		_, ok := nodes[expected]
		require.Truef(t, ok, "missing directly listed object %s", expected)
	}

	namespaceID := entities.ComputeEntityID(entities.KindNamespace, "test", "demo")
	contains := edgesByRelation(snapshot.Graph.Edges, topology.RelContains)
	targets := map[entities.EntityID]bool{}
	for _, edge := range contains {
		if edge.Source == namespaceID {
			targets[edge.Target] = true
		}
	}
	for _, expected := range []entities.EntityID{
		entities.ComputeEntityID(entities.KindPod, "test", "demo", "standalone"),
		entities.ComputeEntityID(entities.KindService, "test", "demo", "external-db"),
		entities.ComputeEntityID(entities.KindIngress, "test", "demo", "public"),
	} {
		require.Truef(t, targets[expected], "missing Namespace containment for %s", expected)
	}
}

func TestModuleBuildsWorkloadChainsFromOwnerReferences(t *testing.T) {
	controller := true
	client := fake.NewClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", UID: "deployment-uid"}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "demo", UID: "replicaset-uid", OwnerReferences: []metav1.OwnerReference{{UID: "deployment-uid", Controller: &controller}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "demo", OwnerReferences: []metav1.OwnerReference{{UID: "replicaset-uid", Controller: &controller}}}},
	)
	graphs := persistence.NewInMemoryGraphStore()
	module := New(client, "test", graphs, persistence.NewInMemoryEntityRegistry())
	require.NoError(t, module.Refresh(context.Background()))

	snapshot, err := graphs.GetLatest(context.Background())
	require.NoError(t, err)
	deploymentID := entities.NewWorkload("test", "demo", "api", entities.WorkloadDeployment, nil).ID()
	replicaSetID := entities.NewWorkload("test", "demo", "api-rs", entities.WorkloadReplicaSet, nil).ID()
	podID := entities.NewPod("test", "demo", "api-pod", nil).ID()
	contains := edgesByRelation(snapshot.Graph.Edges, topology.RelContains)
	require.True(t, hasEdge(contains, deploymentID, replicaSetID))
	require.True(t, hasEdge(contains, replicaSetID, podID))
}

func hasEdge(edges []topology.Edge, source, target entities.EntityID) bool {
	for _, edge := range edges {
		if edge.Source == source && edge.Target == target {
			return true
		}
	}
	return false
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
