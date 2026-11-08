package kubernetes

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/detection"
)

// DiscoveryAdapter gathers Kubernetes API observations once and passes them to
// the compiled source modules. It owns API transport and failure propagation,
// not source-specific detection rules. Structural inventory remains owned by
// kubernetes.structure at backend runtime.
type DiscoveryAdapter struct {
	client            kubernetes.Interface
	clusterName       string
	now               func() time.Time
	targetNamespaces  []string
	excludedNamespace string
}

type DiscoveryOption func(*DiscoveryAdapter)

func WithTargetNamespaces(namespaces ...string) DiscoveryOption {
	return func(d *DiscoveryAdapter) { d.targetNamespaces = append([]string(nil), namespaces...) }
}

// WithExcludedNamespace excludes Karthos' own installation from source detection.
func WithExcludedNamespace(namespace string) DiscoveryOption {
	return func(d *DiscoveryAdapter) { d.excludedNamespace = namespace }
}

func NewDiscoveryAdapter(client kubernetes.Interface, clusterName string, opts ...DiscoveryOption) *DiscoveryAdapter {
	adapter := &DiscoveryAdapter{client: client, clusterName: clusterName, now: time.Now}
	for _, option := range opts {
		option(adapter)
	}
	return adapter
}

// Discover fails when a required Kubernetes API list is denied rather than
// treating that failure as an unavailable source capability.
func (d *DiscoveryAdapter) Discover(ctx context.Context) (*profile.ClusterProfile, error) {
	inventory, err := d.inventory(ctx)
	if err != nil {
		return nil, err
	}
	capabilities, err := sources.DefaultRegistry().Detect(inventory)
	if err != nil {
		return nil, fmt.Errorf("detect compiled source modules: %w", err)
	}
	result := &profile.ClusterProfile{
		ClusterID:    entities.ComputeEntityID(entities.KindCluster, d.clusterName),
		ClusterName:  d.clusterName,
		DiscoveredAt: d.now().UTC(),
		Capabilities: capabilities,
	}
	result.Sort()
	return result, nil
}

func (d *DiscoveryAdapter) inventory(ctx context.Context) (detection.Inventory, error) {
	if d.client == nil {
		return detection.Inventory{}, fmt.Errorf("kubernetes client is required")
	}
	nodes, err := d.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return detection.Inventory{}, fmt.Errorf("list nodes: %w", err)
	}
	services, err := d.client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return detection.Inventory{}, fmt.Errorf("list services: %w", err)
	}
	namespaces, err := d.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return detection.Inventory{}, fmt.Errorf("list namespaces: %w", err)
	}
	pods, err := d.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return detection.Inventory{}, fmt.Errorf("list pods: %w", err)
	}
	webhooks, err := d.client.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return detection.Inventory{}, fmt.Errorf("list mutatingwebhookconfigurations: %w", err)
	}
	return detection.Inventory{
		ClusterName:                   d.clusterName,
		TargetNamespaces:              append([]string(nil), d.targetNamespaces...),
		Nodes:                         nodes.Items,
		Services:                      withoutNamespace(services.Items, d.excludedNamespace, func(item *corev1.Service) string { return item.Namespace }),
		Namespaces:                    withoutNamespace(namespaces.Items, d.excludedNamespace, func(item *corev1.Namespace) string { return item.Name }),
		Pods:                          withoutNamespace(pods.Items, d.excludedNamespace, func(item *corev1.Pod) string { return item.Namespace }),
		MutatingWebhookConfigurations: webhooks.Items,
	}, nil
}

func withoutNamespace[T any](items []T, excluded string, namespace func(*T) string) []T {
	if excluded == "" {
		return items
	}
	filtered := make([]T, 0, len(items))
	for i := range items {
		if namespace(&items[i]) != excluded {
			filtered = append(filtered, items[i])
		}
	}
	return filtered
}
