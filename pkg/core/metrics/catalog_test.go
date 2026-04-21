package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogIncludesRequiredMetrics(t *testing.T) {
	required := map[string]struct{}{
		"k8s.pod.cpu.usage":                     {},
		"k8s.pod.memory.working_set":            {},
		"system.disk.io.read":                   {},
		"system.disk.io.write":                  {},
		"cpu_capacity_millicores":               {},
		"cpu_cores":                             {},
		"cpu_clock_hz":                          {},
		"memory_capacity_bytes":                 {},
		"disk_capacity_bytes":                   {},
		"net_in_bandwidth_bps":                  {},
		"net_out_bandwidth_bps":                 {},
		"servicegraph.request.count":            {},
		"net_rx_bytes":                          {},
		"net_tx_bytes":                          {},
		"http.server.request.duration":          {},
		"servicegraph.request.duration":         {},
		"rpc.server.duration":                   {},
		"rpc.server.request.count":              {},
		"servicegraph.request.error.count":      {},
		"servicegraph.request.error.ratio":      {},
		"k8s.container.cpu.request":             {},
		"k8s.container.cpu.limit":               {},
		"k8s.container.memory.request":          {},
		"k8s.container.memory.limit":            {},
		"k8s.replicaset.desired":                {},
		"system.network.io.receive":             {},
		"system.network.io.transmit":            {},
		"k8s.volume.available":                  {},
		"k8s.volume.capacity":                   {},
		"k8s.volume.inodes.free":                {},
		"k8s.volume.inodes.used":                {},
		"system.filesystem.usage":               {},
		"system.filesystem.utilization":         {},
		"system.cpu.load_average.1m":            {},
		"system.cpu.load_average.5m":            {},
		"system.cpu.load_average.15m":           {},
		"system.paging.usage":                   {},
		"system.paging.operations":              {},
		"k8s.pod.phase":                         {},
		"k8s.namespace.phase":                   {},
		"k8s.node.condition.ready":              {},
		"k8s.node.condition.memory_pressure":    {},
		"k8s.node.condition.disk_pressure":      {},
		"k8s.node.allocatable_cpu":              {},
		"k8s.node.allocatable_memory":           {},
		"k8s.deployment.available":              {},
		"k8s.daemonset.desired_scheduled_nodes": {},
		"k8s.daemonset.ready_nodes":             {},
		"k8s.statefulset.desired_pods":          {},
		"k8s.statefulset.ready_pods":            {},
	}

	for _, metric := range DefaultCatalog() {
		delete(required, metric.Name)
	}

	require.Empty(t, required)
}

func TestDefaultCatalogMetricNamesAreUnique(t *testing.T) {
	seen := make(map[string]struct{})

	for _, metric := range DefaultCatalog() {
		_, exists := seen[metric.Name]
		require.Falsef(t, exists, "duplicate metric in catalog: %s", metric.Name)
		seen[metric.Name] = struct{}{}
	}
}
