package structure

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/topology"
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
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: "demo", Labels: map[string]string{"app": "api"}}, Spec: corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "api"}}}},
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

	snapshot, err := graphStore.Materialize(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelExposes), 1)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelRoutesTo), 1)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelContains), 3)
	require.Len(t, edgesByRelation(snapshot.Graph.Edges, topology.RelUses), 1)

	_, err = entitiesStore.GetEntity(context.Background(), entities.NewWorkload("test", "demo", "api", entities.WorkloadDeployment, nil).ID())
	require.NoError(t, err)
	_, err = entitiesStore.GetEntity(context.Background(), entities.ComputeEntityID(entities.KindConfigMap, "test", "demo", "api-config"))
	require.NoError(t, err)
	containerID := entities.ComputeEntityID(entities.KindContainer, "test", "demo", "api-1", "api")
	require.True(t, hasEdge(edgesByRelation(snapshot.Graph.Edges, topology.RelContains), entities.NewPod("test", "demo", "api-1", nil).ID(), containerID))
}

func TestTerminalPodRemainsInHistoryAfterDeletion(t *testing.T) {
	ctx := context.Background()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "demo"}, Status: corev1.PodStatus{Phase: corev1.PodFailed}}
	client := fake.NewClientset(pod)
	store := persistence.NewInMemoryGraphStore()
	module := New(client, "test", store, nil)
	first := time.Now().UTC().Add(-time.Minute)
	module.now = func() time.Time { return first }
	require.NoError(t, module.Refresh(ctx))
	_, err := store.Materialize(ctx, first)
	require.NoError(t, err)
	require.NoError(t, client.CoreV1().Pods("demo").Delete(ctx, "job-1", metav1.DeleteOptions{}))
	second := first.Add(time.Minute)
	module.now = func() time.Time { return second }
	require.NoError(t, module.Refresh(ctx))
	_, err = store.Materialize(ctx, second)
	require.NoError(t, err)
	historical, err := store.GetAt(ctx, first)
	require.NoError(t, err)
	require.NotNil(t, historical)
	id := entities.ComputeEntityID(entities.KindPod, "test", "demo", "job-1")
	require.True(t, hasNode(historical.Graph.Nodes, id))
	for _, node := range historical.Graph.Nodes {
		if node.ID == id {
			require.Equal(t, "Failed", node.Labels["k8s.pod.phase"])
		}
	}
	current, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.False(t, hasNode(current.Graph.Nodes, id))
}

func hasNode(nodes []topology.Node, id entities.EntityID) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
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

	snapshot, err := graphStore.Materialize(context.Background(), time.Now().UTC())
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

	snapshot, err := graphs.Materialize(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	deploymentID := entities.NewWorkload("test", "demo", "api", entities.WorkloadDeployment, nil).ID()
	replicaSetID := entities.NewWorkload("test", "demo", "api-rs", entities.WorkloadReplicaSet, nil).ID()
	podID := entities.NewPod("test", "demo", "api-pod", nil).ID()
	contains := edgesByRelation(snapshot.Graph.Edges, topology.RelContains)
	require.True(t, hasEdge(contains, deploymentID, replicaSetID))
	require.True(t, hasEdge(contains, replicaSetID, podID))
}

func TestModuleLinksServiceToTheHighestProvenMatchingWorkloadOwner(t *testing.T) {
	controller := true
	client := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", UID: "deployment-uid"},
			Spec:       appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}}}},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: "api-rs", Namespace: "demo", UID: "replicaset-uid", OwnerReferences: []metav1.OwnerReference{{UID: "deployment-uid", Controller: &controller}}},
			Spec:       appsv1.ReplicaSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}}}},
		},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"}, Spec: corev1.ServiceSpec{Selector: map[string]string{"app": "api"}}},
	)
	graphs := persistence.NewInMemoryGraphStore()
	module := New(client, "test", graphs, persistence.NewInMemoryEntityRegistry())
	require.NoError(t, module.Refresh(context.Background()))

	snapshot, err := graphs.Materialize(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	serviceID := entities.NewService("test", "demo", "api", nil).ID()
	deploymentID := entities.NewWorkload("test", "demo", "api", entities.WorkloadDeployment, nil).ID()
	workloadTargets := map[entities.EntityID]bool{}
	for _, edge := range edgesByRelation(snapshot.Graph.Edges, topology.RelExposes) {
		if edge.Source == serviceID {
			workloadTargets[edge.Target] = true
		}
	}
	require.Equal(t, map[entities.EntityID]bool{deploymentID: true}, workloadTargets)
}

func TestModuleKeepsHPANamespaceContained(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "api-scale", Namespace: "demo"}},
	)
	graphs := persistence.NewInMemoryGraphStore()
	module := New(client, "test", graphs, persistence.NewInMemoryEntityRegistry())
	require.NoError(t, module.Refresh(context.Background()))

	snapshot, err := graphs.Materialize(context.Background(), time.Now().UTC())
	require.NoError(t, err)
	namespaceID := entities.ComputeEntityID(entities.KindNamespace, "test", "demo")
	hpaID := entities.NewHPA("test", "demo", "api-scale", nil).ID()
	require.True(t, hasEdge(edgesByRelation(snapshot.Graph.Edges, topology.RelContains), namespaceID, hpaID))
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
