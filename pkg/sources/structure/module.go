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
	graphs   persistence.StructuralSnapshotStore
	entities persistence.EntityRegistry
	now      func() time.Time
}

func New(client kubernetes.Interface, cluster string, graphs persistence.StructuralSnapshotStore, entities persistence.EntityRegistry) *Module {
	return &Module{client: client, cluster: cluster, graphs: graphs, entities: entities, now: time.Now}
}

// Refresh observes the current Kubernetes API state and appends one complete
// structural snapshot. Runtime telemetry topology continues through its own
// observation path.
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
	now := m.now().UTC()
	return m.graphs.PutStructuralSnapshot(ctx, topology.Snapshot{
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
	workloads  []workloadRecord
	hpas       []entities.HPA
	quotas     []entities.ResourceQuota
	pods       []podRecord
	services   []serviceRecord
	ingresses  []ingressRecord
}

type workloadRecord struct {
	entity            entities.Workload
	podTemplateLabels map[string]string
	configMapRefs     []string
	secretRefs        []string
	pvcRefs           []string
}

type podRecord struct {
	entity     entities.Pod
	workloadID entities.EntityID
}

type serviceRecord struct {
	entity   entities.Service
	selector map[string]string
}

type ingressRecord struct {
	entity          entities.Ingress
	backendServices []string
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
	sort.Slice(result.workloads, func(i, j int) bool { return result.workloads[i].entity.ID() < result.workloads[j].entity.ID() })
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
		result.pods = append(result.pods, podRecord{entity: pod, workloadID: matchingWorkload(pod, result.workloads)})
	}
	for _, item := range services.Items {
		result.services = append(result.services, serviceRecord{entity: entities.NewService(m.cluster, item.Namespace, item.Name, item.Labels), selector: clone(item.Spec.Selector)})
	}
	for _, item := range ingresses.Items {
		result.ingresses = append(result.ingresses, ingressRecord{entity: entities.NewIngress(m.cluster, item.Namespace, item.Name, item.Labels), backendServices: ingressBackends(item)})
	}
	return result, nil
}

func workloadFromDeployment(cluster string, item appsv1.Deployment) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadDeployment, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	return newWorkloadRecord(w, item.Spec.Template)
}
func workloadFromStatefulSet(cluster string, item appsv1.StatefulSet) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadStatefulSet, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	return newWorkloadRecord(w, item.Spec.Template)
}
func workloadFromDaemonSet(cluster string, item appsv1.DaemonSet) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadDaemonSet, item.Labels)
	return newWorkloadRecord(w, item.Spec.Template)
}
func workloadFromReplicaSet(cluster string, item appsv1.ReplicaSet) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadReplicaSet, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	return newWorkloadRecord(w, item.Spec.Template)
}
func workloadFromJob(cluster string, item batchv1.Job) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadJob, item.Labels)
	if item.Spec.Completions != nil {
		w.Replicas = *item.Spec.Completions
	}
	return newWorkloadRecord(w, item.Spec.Template)
}
func workloadFromCronJob(cluster string, item batchv1.CronJob) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadCronJob, item.Labels)
	return newWorkloadRecord(w, item.Spec.JobTemplate.Spec.Template)
}
func workloadFromRC(cluster string, item corev1.ReplicationController) workloadRecord {
	w := entities.NewWorkload(cluster, item.Namespace, item.Name, entities.WorkloadReplicationController, item.Labels)
	if item.Spec.Replicas != nil {
		w.Replicas = *item.Spec.Replicas
	}
	if item.Spec.Template == nil {
		return workloadRecord{entity: w}
	}
	template := *item.Spec.Template
	refs := templateRefs(template)
	return workloadRecord{entity: w, podTemplateLabels: clone(template.Labels), configMapRefs: refs.configMaps, secretRefs: refs.secrets, pvcRefs: refs.pvcs}
}

func newWorkloadRecord(entity entities.Workload, template corev1.PodTemplateSpec) workloadRecord {
	references := templateRefs(template)
	return workloadRecord{entity: entity, podTemplateLabels: clone(template.Labels), configMapRefs: references.configMaps, secretRefs: references.secrets, pvcRefs: references.pvcs}
}

type refs struct {
	configMaps []string
	secrets    []string
	pvcs       []string
}

func templateRefs(template corev1.PodTemplateSpec) refs {
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
	return refs{configMaps: sorted(configMaps), secrets: sorted(secrets), pvcs: sorted(pvcs)}
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
		entity := workload.entity
		labels := workloadLabels(entity)
		add(node(entity.ID(), entities.KindWorkload, m.cluster, entity.Namespace, entity.Name, labels))
		put(entities.KindWorkload, entity.ID(), entity.Namespace, entity.Name, labels)
		namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, entity.Namespace)
		add(node(namespaceID, entities.KindNamespace, m.cluster, entity.Namespace, entity.Namespace, nil))
		link(namespaceID, entity.ID(), topology.RelContains)
		for _, name := range workload.configMapRefs {
			id := entities.ComputeEntityID(entities.KindConfigMap, m.cluster, entity.Namespace, name)
			add(node(id, entities.KindConfigMap, m.cluster, entity.Namespace, name, nil))
			put(entities.KindConfigMap, id, entity.Namespace, name, nil)
			link(entity.ID(), id, topology.RelUses)
		}
		for _, name := range workload.secretRefs {
			id := entities.ComputeEntityID(entities.KindSecret, m.cluster, entity.Namespace, name)
			add(node(id, entities.KindSecret, m.cluster, entity.Namespace, name, nil))
			put(entities.KindSecret, id, entity.Namespace, name, nil)
			link(entity.ID(), id, topology.RelUses)
		}
		for _, name := range workload.pvcRefs {
			id := entities.ComputeEntityID(entities.KindPVC, m.cluster, entity.Namespace, name)
			add(node(id, entities.KindPVC, m.cluster, entity.Namespace, name, nil))
			put(entities.KindPVC, id, entity.Namespace, name, nil)
			link(entity.ID(), id, topology.RelMounts)
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
	workloadByID := map[entities.EntityID]workloadRecord{}
	for _, workload := range inventory.workloads {
		workloadByID[workload.entity.ID()] = workload
	}
	for _, pod := range inventory.pods {
		entity := pod.entity
		add(node(entity.ID(), entities.KindPod, m.cluster, entity.Namespace, entity.Name, entity.Labels()))
		put(entities.KindPod, entity.ID(), entity.Namespace, entity.Name, entity.Labels())
		if workload, ok := workloadByID[pod.workloadID]; ok {
			add(node(workload.entity.ID(), entities.KindWorkload, m.cluster, workload.entity.Namespace, workload.entity.Name, workloadLabels(workload.entity)))
			link(workload.entity.ID(), entity.ID(), topology.RelContains)
			continue
		}
		namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, entity.Namespace)
		add(node(namespaceID, entities.KindNamespace, m.cluster, entity.Namespace, entity.Namespace, nil))
		link(namespaceID, entity.ID(), topology.RelContains)
	}
	services := map[string]entities.Service{}
	for _, service := range inventory.services {
		entity := service.entity
		services[entity.Namespace+"/"+entity.Name] = entity
		add(node(entity.ID(), entities.KindService, m.cluster, entity.Namespace, entity.Name, entity.Labels()))
		put(entities.KindService, entity.ID(), entity.Namespace, entity.Name, entity.Labels())
		matched := false
		for _, workload := range inventory.workloads {
			if entity.Namespace == workload.entity.Namespace && matches(service.selector, workload.podTemplateLabels) {
				matched = true
				add(node(workload.entity.ID(), entities.KindWorkload, m.cluster, workload.entity.Namespace, workload.entity.Name, workloadLabels(workload.entity)))
				link(entity.ID(), workload.entity.ID(), topology.RelExposes)
			}
		}
		if !matched {
			namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, entity.Namespace)
			add(node(namespaceID, entities.KindNamespace, m.cluster, entity.Namespace, entity.Namespace, nil))
			link(namespaceID, entity.ID(), topology.RelContains)
		}
	}
	for _, ingress := range inventory.ingresses {
		entity := ingress.entity
		add(node(entity.ID(), entities.KindIngress, m.cluster, entity.Namespace, entity.Name, entity.Labels()))
		put(entities.KindIngress, entity.ID(), entity.Namespace, entity.Name, entity.Labels())
		routed := false
		for _, backend := range ingress.backendServices {
			service, ok := services[entity.Namespace+"/"+backend]
			if !ok {
				continue
			}
			add(node(service.ID(), entities.KindService, m.cluster, service.Namespace, service.Name, service.Labels()))
			link(entity.ID(), service.ID(), topology.RelRoutesTo)
			routed = true
		}
		if !routed {
			namespaceID := entities.ComputeEntityID(entities.KindNamespace, m.cluster, entity.Namespace)
			add(node(namespaceID, entities.KindNamespace, m.cluster, entity.Namespace, entity.Namespace, nil))
			link(namespaceID, entity.ID(), topology.RelContains)
		}
	}
	return sortedEdges(edges), sortedNodes(nodes), sortedEntities(stored)
}

func terminating(pod corev1.Pod) bool {
	return pod.DeletionTimestamp != nil || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}
func matchingWorkload(pod entities.Pod, workloads []workloadRecord) entities.EntityID {
	for _, workload := range workloads {
		if pod.Namespace == workload.entity.Namespace && matches(workload.podTemplateLabels, pod.Labels()) {
			return workload.entity.ID()
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

func ingressBackends(ingress networkingv1.Ingress) []string {
	seen, out := map[string]struct{}{}, []string{}
	add := func(service *networkingv1.IngressServiceBackend) {
		if service == nil {
			return
		}
		if _, ok := seen[service.Name]; ok {
			return
		}
		seen[service.Name] = struct{}{}
		out = append(out, service.Name)
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
