package kubernetes

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
	"github.com/erickivel/oksg/pkg/sources/detection"
)

// DiscoveryAdapter gathers Kubernetes API observations once and passes them to
// the compiled source modules. It owns API transport and failure propagation,
// not source-specific detection rules. Structural inventory remains owned by
// kubernetes.structure at backend runtime.
type DiscoveryAdapter struct {
	client           kubernetes.Interface
	clusterName      string
	now              func() time.Time
	targetNamespaces []string
}

type DiscoveryOption func(*DiscoveryAdapter)

func WithTargetNamespaces(namespaces ...string) DiscoveryOption {
	return func(d *DiscoveryAdapter) { d.targetNamespaces = append([]string(nil), namespaces...) }
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
		return detection.Inventory{}, fmt.Errorf("Kubernetes client is required")
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
		Services:                      services.Items,
		Namespaces:                    namespaces.Items,
		Pods:                          pods.Items,
		MutatingWebhookConfigurations: webhooks.Items,
	}, nil
}
