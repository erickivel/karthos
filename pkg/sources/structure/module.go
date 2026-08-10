// Package structure owns Kubernetes API topology refreshes. It deliberately
// does not consume the capability profile: that profile is capability-only, while
// the topology is a live, directly observed runtime projection.
package structure

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

const provenance = "k8s-api"

// Module turns live Kubernetes structure into the canonical topology snapshot.
// It is intentionally not a telemetry source and never participates in metric
// source arbitration.
type Module struct {
	client   kubernetes.Interface
	cluster  string
	graphs   persistence.GraphStore
	entities persistence.EntityRegistry
	now      func() time.Time
}

func New(client kubernetes.Interface, cluster string, graphs persistence.GraphStore, entities persistence.EntityRegistry) *Module {
	return &Module{client: client, cluster: cluster, graphs: graphs, entities: entities, now: time.Now}
}

// Refresh observes the current Kubernetes API state and replaces the current
// structural snapshot. Runtime telemetry topology continues through its own
// ingestion path.
func (m *Module) Refresh(ctx context.Context) error {
	if m.client == nil {
		return fmt.Errorf("structure module Kubernetes client is required")
	}
	if m.graphs == nil {
		return fmt.Errorf("structure module graph store is required")
	}

	inventory, err := m.collect(ctx)
	if err != nil {
		return err
	}
	edges, nodes, stored := m.assemble(inventory)
	if m.entities != nil {
		for _, entity := range stored {
			if err := m.entities.UpsertEntity(ctx, entity); err != nil {
				return fmt.Errorf("upsert structural entity %s: %w", entity.ID(), err)
			}
		}
	}
	if len(edges) == 0 && len(nodes) == 0 {
		return nil
	}
	now := m.now().UTC()
	return m.graphs.PutSnapshot(ctx, topology.Snapshot{
		Version:   schema.CurrentVersion,
		Graph:     topology.Graph{Nodes: nodes, Edges: edges, Timestamp: now},
		From:      now,
		To:        now,
		CreatedAt: now,
	})
}

type inventory struct {
	nodes      []entities.Node
	namespaces []entities.Namespace
	workloads  []entities.Workload
	hpas       []entities.HPA
	quotas     []entities.ResourceQuota
	pods       []entities.Pod
	services   []entities.Service
	ingresses  []entities.Ingress
}

func (m *Module) collect(ctx context.Context) (inventory, error) {
	// Keep each List explicit: required RBAC remains auditable and a denied API
	// operation is a refresh failure rather than an incomplete structural view.
	nodes, err := m.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list nodes: %w", err)
	}
	namespaces, err := m.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list namespaces: %w", err)
	}
	deployments, err := m.client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list deployments: %w", err)
	}
	statefulSets, err := m.client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list statefulsets: %w", err)
	}
	daemonSets, err := m.client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list daemonsets: %w", err)
	}
	replicaSets, err := m.client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list replicasets: %w", err)
	}
	jobs, err := m.client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list jobs: %w", err)
	}
	cronJobs, err := m.client.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list cronjobs: %w", err)
	}
	rcs, err := m.client.CoreV1().ReplicationControllers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list replicationcontrollers: %w", err)
	}
	hpas, err := m.client.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list horizontalpodautoscalers: %w", err)
	}
	quotas, err := m.client.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list resourcequotas: %w", err)
	}
	pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list pods: %w", err)
	}
	services, err := m.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list services: %w", err)
	}
	ingresses, err := m.client.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return inventory{}, fmt.Errorf("list ingresses: %w", err)
	}

	result := inventory{}
	for _, item := range nodes.Items {
		result.nodes = append(result.nodes, entities.NewNode(m.cluster, item.Name, item.Labels))
	}
	for _, item := range namespaces.Items {
		result.namespaces = append(result.namespaces, entities.NewNamespace(m.cluster, item.Name, item.Labels))
	}
	for _, item := range deployments.Items {
		result.workloads = append(result.workloads, workloadFromDeployment(m.cluster, item))
	}
	for _, item := range statefulSets.Items {
		result.workloads = append(result.workloads, workloadFromStatefulSet(m.cluster, item))
	}
	for _, item := range daemonSets.Items {
		result.workloads = append(result.workloads, workloadFromDaemonSet(m.cluster, item))
	}
	for _, item := range replicaSets.Items {
		result.workloads = append(result.workloads, workloadFromReplicaSet(m.cluster, item))
	}
	for _, item := range jobs.Items {
		result.workloads = append(result.workloads, workloadFromJob(m.cluster, item))
	}
	for _, item := range cronJobs.Items {
		result.workloads = append(result.workloads, workloadFromCronJob(m.cluster, item))
	}
	for _, item := range rcs.Items {
		result.workloads = append(result.workloads, workloadFromRC(m.cluster, item))
	}
	sort.Slice(result.workloads, func(i, j int) bool { return result.workloads[i].ID() < result.workloads[j].ID() })
	for _, item := range hpas.Items {
		result.hpas = append(result.hpas, entities.NewHPA(m.cluster, item.Namespace, item.Name, item.Labels))
	}
	for _, item := range quotas.Items {
		result.quotas = append(result.quotas, entities.NewResourceQuota(m.cluster, item.Namespace, item.Name, item.Labels))
	}
	for _, item := range pods.Items {
		if terminating(item) {
			continue
		}
		pod := entities.NewPod(m.cluster, item.Namespace, item.Name, item.Labels)
		pod.WorkloadID = matchingWorkload(pod, result.workloads)
		result.pods = append(result.pods, pod)
	}
	for _, item := range services.Items {
		result.services = append(result.services, entities.NewService(m.cluster, item.Namespace, item.Name, item.Spec.Selector, item.Labels))
	}
	for _, item := range ingresses.Items {
		ingress := entities.NewIngress(m.cluster, item.Namespace, item.Name, "", item.Labels)
		ingress.BackendServices = ingressBackends(item)
		result.ingresses = append(result.ingresses, ingress)
	}
	return result, nil
}

func workloadFromDeployment(cluster string, item appsv1.Deployment) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadDeployment, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	setTemplate(&w, item.Spec.Template)
	return w
}
func workloadFromStatefulSet(cluster string, item appsv1.StatefulSet) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadStatefulSet, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	setTemplate(&w, item.Spec.Template)
	return w
}
func workloadFromDaemonSet(cluster string, item appsv1.DaemonSet) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadDaemonSet, item.Labels)
	setTemplate(&w, item.Spec.Template)
	return w
}
func workloadFromReplicaSet(cluster string, item appsv1.ReplicaSet) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadReplicaSet, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	setTemplate(&w, item.Spec.Template)
	return w
}
func workloadFromJob(cluster string, item batchv1.Job) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadJob, item.Labels)
	if item.Spec.Completions != nil {
		w.Replicas = *item.Spec.Completions
	}
	setTemplate(&w, item.Spec.Template)
	return w
}
func workloadFromCronJob(cluster string, item batchv1.CronJob) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadCronJob, item.Labels)
	setTemplate(&w, item.Spec.JobTemplate.Spec.Template)
	return w
}
func workloadFromRC(cluster string, item corev1.ReplicationController) entities.Workload {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadReplicationController, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	if item.Spec.Template != nil {
		setTemplate(&w, *item.Spec.Template)
	}
	return w
}

func setTemplate(workload *entities.Workload, template corev1.PodTemplateSpec) {
	workload.PodTemplateLabels = clone(template.Labels)
	configMaps, secrets, pvcs := map[string]struct{}{}, map[string]struct{}{}, map[string]struct{}{}
	for _, volume := range template.Spec.Volumes {
		if volume.ConfigMap != nil {
			configMaps[volume.ConfigMap.Name] = struct{}{}
		}
		if volume.Secret != nil {
			secrets[volume.Secret.SecretName] = struct{}{}
		}
		if volume.PersistentVolumeClaim != nil {
			pvcs[volume.PersistentVolumeClaim.ClaimName] = struct{}{}
		}
	}
	for _, container := range template.Spec.Containers {
		for _, source := range container.EnvFrom {
			if source.ConfigMapRef != nil {
				configMaps[source.ConfigMapRef.Name] = struct{}{}
			}
			if source.SecretRef != nil {
				secrets[source.SecretRef.Name] = struct{}{}
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}
			if env.ValueFrom.ConfigMapKeyRef != nil {
				configMaps[env.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
			}
			if env.ValueFrom.SecretKeyRef != nil {
				secrets[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
			}
		}
	}
	workload.ConfigMapRefs, workload.SecretRefs, workload.PVCRefs = sorted(configMaps), sorted(secrets), sorted(pvcs)
}

func (m *Module) assemble(inventory inventory) ([]topology.Edge, []topology.Node, []persistence.StoredEntity) {
	now := m.now().UTC()
	edges := map[string]topology.Edge{}
	nodes := map[entities.EntityID]topology.Node{}
	stored := map[entities.EntityID]persistence.StoredEntity{}
	add := func(node topology.Node) {
		if _, ok := nodes[node.ID]; !ok {
			nodes[node.ID] = node
		}
	}
	put := func(kind entities.EntityKind, id entities.EntityID, namespace, name string, labels map[string]string) {
		stored[id] = persistence.StoredEntity{EntityID: id, EntityKind: kind, Cluster: m.cluster, Namespace: namespace, Name: name, Metadata: metadata(m.cluster, namespace, name, labels)}
	}
	link := func(source, target entities.EntityID, relation topology.RelationType) {
		edge := topology.NewEdge(source, target, relation, now)
		edge.Provenance = []string{provenance}
		edges[edge.ID] = edge
	}
	for _, item := range inventory.nodes {
		add(node(item.ID(), entities.KindNode, m.cluster, "", item.Name, item.Labels()))
		put(entities.KindNode, item.ID(), "", item.Name, item.Labels())
	}
	for _, item := range inventory.namespaces {
		add(node(item.ID(), entities.KindNamespace, m.cluster, item.Name, item.Name, item.Labels()))
		put(entities.KindNamespace, item.ID(), item.Name, item.Name, item.Labels())
	}
	for _, workload := range inventory.workloads {
		labels := workloadLabels(workload)
		add(node(workload.ID(), entities.KindWorkload, m.cluster, workload.Namespace, workload.Name, labels))
		put(entities.KindWorkload, workload.ID(), workload.Namespace, workload.Name, labels)
		namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, workload.Namespace)
		add(node(namespaceID, entities.KindNamespace, m.cluster, workload.Namespace, workload.Namespace, nil))
		link(namespaceID, workload.ID(), topology.RelContains)
		for _, name := range workload.ConfigMapRefs {
			id := entities.ComputeEntityID(entities.KindConfigMap, m.cluster, workload.Namespace, name)
			add(node(id, entities.KindConfigMap, m.cluster, workload.Namespace, name, nil))
			put(entities.KindConfigMap, id, workload.Namespace, name, nil)
			link(workload.ID(), id, topology.RelUses)
		}
		for _, name := range workload.SecretRefs {
			id := entities.ComputeEntityID(entities.KindSecret, m.cluster, workload.Namespace, name)
			add(node(id, entities.KindSecret, m.cluster, workload.Namespace, name, nil))
			put(entities.KindSecret, id, workload.Namespace, name, nil)
			link(workload.ID(), id, topology.RelUses)
		}
		for _, name := range workload.PVCRefs {
			id := entities.ComputeEntityID(entities.KindPVC, m.cluster, workload.Namespace, name)
			add(node(id, entities.KindPVC, m.cluster, workload.Namespace, name, nil))
			put(entities.KindPVC, id, workload.Namespace, name, nil)
			link(workload.ID(), id, topology.RelMounts)
		}
	}
	for _, hpa := range inventory.hpas {
		add(node(hpa.ID(), entities.KindHPA, m.cluster, hpa.Namespace, hpa.Name, hpa.Labels()))
		put(entities.KindHPA, hpa.ID(), hpa.Namespace, hpa.Name, hpa.Labels())
		namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, hpa.Namespace)
		add(node(namespaceID, entities.KindNamespace, m.cluster, hpa.Namespace, hpa.Namespace, nil))
		link(namespaceID, hpa.ID(), topology.RelContains)
	}
	for _, quota := range inventory.quotas {
		add(node(quota.ID(), entities.KindResourceQuota, m.cluster, quota.Namespace, quota.Name, quota.Labels()))
		put(entities.KindResourceQuota, quota.ID(), quota.Namespace, quota.Name, quota.Labels())
		namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, quota.Namespace)
		add(node(namespaceID, entities.KindNamespace, m.cluster, quota.Namespace, quota.Namespace, nil))
		link(namespaceID, quota.ID(), topology.RelContains)
	}
	workloadByID := map[entities.EntityID]entities.Workload{}
	for _, workload := range inventory.workloads {
		workloadByID[workload.ID()] = workload
	}
	for _, pod := range inventory.pods {
		if pod.WorkloadID == "" {
			continue
		}
		workload, ok := workloadByID[pod.WorkloadID]
		if !ok {
			continue
		}
		add(node(pod.ID(), entities.KindPod, m.cluster, pod.Namespace, pod.Name, pod.Labels()))
		put(entities.KindPod, pod.ID(), pod.Namespace, pod.Name, pod.Labels())
		add(node(workload.ID(), entities.KindWorkload, m.cluster, workload.Namespace, workload.Name, workloadLabels(workload)))
		link(workload.ID(), pod.ID(), topology.RelContains)
	}
	services := map[string]entities.Service{}
	for _, service := range inventory.services {
		services[service.Namespace+"/"+service.Name] = service
		put(entities.KindService, service.ID(), service.Namespace, service.Name, service.Labels())
		for _, workload := range inventory.workloads {
			if service.Namespace == workload.Namespace && matches(service.Selector, workload.PodTemplateLabels) {
				add(node(service.ID(), entities.KindService, m.cluster, service.Namespace, service.Name, service.Labels()))
				add(node(workload.ID(), entities.KindWorkload, m.cluster, workload.Namespace, workload.Name, workloadLabels(workload)))
				link(service.ID(), workload.ID(), topology.RelExposes)
			}
		}
	}
	for _, ingress := range inventory.ingresses {
		put(entities.KindIngress, ingress.ID(), ingress.Namespace, ingress.Name, ingress.Labels())
		for _, backend := range ingress.BackendServices {
			service, ok := services[ingress.Namespace+"/"+backend.ServiceName]
			if !ok {
				continue
			}
			add(node(ingress.ID(), entities.KindIngress, m.cluster, ingress.Namespace, ingress.Name, ingress.Labels()))
			add(node(service.ID(), entities.KindService, m.cluster, service.Namespace, service.Name, service.Labels()))
			link(ingress.ID(), service.ID(), topology.RelRoutesTo)
		}
	}
	return sortedEdges(edges), sortedNodes(nodes), sortedEntities(stored)
}

func terminating(pod corev1.Pod) bool {
	return pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}
func matchingWorkload(pod entities.Pod, workloads []entities.Workload) entities.EntityID {
	for _, workload := range workloads {
		if pod.Namespace == workload.Namespace && matches(workload.PodTemplateLabels, pod.Labels()) {
			return workload.ID()
		}
	}
	return ""
}
func matches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}
	return true
}
func clone(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copy := make(map[string]string, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
func sorted(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
func workloadLabels(workload entities.Workload) map[string]string {
	labels := workload.Labels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[entities.WorkloadTypeLabel] = string(workload.Type)
	if workload.Replicas > 0 {
		labels["k8s.workload.replicas"] = strconv.FormatInt(int64(workload.Replicas), 10)
	}
	return labels
}
func metadata(cluster, namespace, name string, labels map[string]string) map[string]string {
	out := clone(labels)
	if out == nil {
		out = map[string]string{}
	}
	out["cluster"] = cluster
	if namespace != "" {
		out["namespace"] = namespace
	}
	if name != "" {
		out["name"] = name
	}
	return out
}
func node(id entities.EntityID, kind entities.EntityKind, cluster, namespace, name string, labels map[string]string) topology.Node {
	return topology.Node{ID: id, Kind: kind, Labels: metadata(cluster, namespace, name, labels)}
}
func sortedEdges(values map[string]topology.Edge) []topology.Edge {
	out := make([]topology.Edge, 0, len(values))
	for _, edge := range values {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func sortedNodes(values map[entities.EntityID]topology.Node) []topology.Node {
	out := make([]topology.Node, 0, len(values))
	for _, node := range values {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func sortedEntities(values map[entities.EntityID]persistence.StoredEntity) []persistence.StoredEntity {
	out := make([]persistence.StoredEntity, 0, len(values))
	for _, entity := range values {
		out = append(out, entity)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

func ingressBackends(ingress networkingv1.Ingress) []entities.IngressBackend {
	seen, out := map[string]struct{}{}, []entities.IngressBackend{}
	add := func(service *networkingv1.IngressServiceBackend) {
		if service == nil {
			return
		}
		key := service.Name + ":" + strconv.FormatInt(int64(service.Port.Number), 10)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, entities.IngressBackend{ServiceName: service.Name, ServicePort: service.Port.Number})
	}
	if ingress.Spec.DefaultBackend != nil {
		add(ingress.Spec.DefaultBackend.Service)
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			add(path.Backend.Service)
		}
	}
	return out
}
