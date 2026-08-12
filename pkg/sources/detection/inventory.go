// Package detection defines the Kubernetes observations consumed by compiled
// source-module capability detectors. It contains data only; Kubernetes API
// access remains owned by the Kubernetes adapter.
package detection

import (
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
)

// Inventory is one successful, consistent-enough set of Kubernetes API lists
// gathered by the discovery adapter. Source modules inspect only the fields
// required by their detector.
type Inventory struct {
	ClusterName                   string
	TargetNamespaces              []string
	Nodes                         []corev1.Node
	Services                      []corev1.Service
	Namespaces                    []corev1.Namespace
	Pods                          []corev1.Pod
	MutatingWebhookConfigurations []admissionv1.MutatingWebhookConfiguration
}
