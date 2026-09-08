package prometheusapp

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/detection"
)

const (
	prometheusAnnotationScrape = "prometheus.io/scrape"
	prometheusAnnotationPath   = "prometheus.io/path"
	prometheusAnnotationPort   = "prometheus.io/port"
	defaultPrometheusPath      = "/metrics"
)

// Detect finds supported annotated Prometheus Service targets.
func Detect(inventory detection.Inventory) profile.SourceCapability {
	targets := make([]profile.PrometheusTarget, 0)
	for _, service := range inventory.Services {
		if target, ok := targetFor(service, inventory.TargetNamespaces); ok {
			targets = append(targets, target)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Namespace != targets[j].Namespace {
			return targets[i].Namespace < targets[j].Namespace
		}
		return targets[i].ServiceName < targets[j].ServiceName
	})
	capability := profile.SourceCapability{SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable}
	if len(targets) > 0 {
		capability.Status = profile.CapabilityAvailable
		capability.Signals = []string{"metric"}
		capability.Evidence.PrometheusTargets = targets
	}
	return capability
}

func targetFor(service corev1.Service, targetNamespaces []string) (profile.PrometheusTarget, bool) {
	if !strings.EqualFold(service.Annotations[prometheusAnnotationScrape], "true") || !targetNamespace(service.Namespace, targetNamespaces) {
		return profile.PrometheusTarget{}, false
	}
	port, err := strconv.ParseInt(service.Annotations[prometheusAnnotationPort], 10, 32)
	if err != nil || port < 1 || port > 65535 {
		return profile.PrometheusTarget{}, false
	}
	declaredPort := int32(port)
	if !declaresPort(service, declaredPort) {
		return profile.PrometheusTarget{}, false
	}
	path := service.Annotations[prometheusAnnotationPath]
	if path == "" {
		path = defaultPrometheusPath
	}
	return profile.PrometheusTarget{ServiceName: service.Name, Namespace: service.Namespace, Port: declaredPort, Path: path}, true
}

func declaresPort(service corev1.Service, port int32) bool {
	for _, declared := range service.Spec.Ports {
		if declared.Port == port {
			return true
		}
	}
	return false
}

func targetNamespace(namespace string, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, target := range targets {
		if target == namespace {
			return true
		}
	}
	return false
}
