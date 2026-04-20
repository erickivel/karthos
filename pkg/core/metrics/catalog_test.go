package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogIncludesRequiredMetrics(t *testing.T) {
	required := map[string]struct{}{
		"cpu_usage_millicores":            {},
		"memory_usage_bytes":              {},
		"disk_read_bytes":                 {},
		"disk_write_bytes":                {},
		"cpu_capacity_millicores":         {},
		"cpu_cores":                       {},
		"cpu_clock_hz":                    {},
		"memory_capacity_bytes":           {},
		"disk_capacity_bytes":             {},
		"net_in_bandwidth_bps":            {},
		"net_out_bandwidth_bps":           {},
		"request_rate_per_second":         {},
		"net_rx_bytes":                    {},
		"net_tx_bytes":                    {},
		"request_duration_nanoseconds":    {},
		"edge_latency_nanoseconds":        {},
		"error_rate_per_second":           {},
		"error_ratio":                     {},
		"cpu_request_millicores":          {},
		"cpu_limit_millicores":            {},
		"memory_request_bytes":            {},
		"memory_limit_bytes":              {},
		"replica_count":                   {},
		"disk_read_bps":                   {},
		"disk_write_bps":                  {},
		"net_rx_bps":                      {},
		"net_tx_bps":                      {},
		"volume_available_bytes":          {},
		"volume_capacity_bytes":           {},
		"volume_inodes_free":              {},
		"volume_inodes_used":              {},
		"filesystem_usage_bytes":          {},
		"filesystem_utilization":          {},
		"cpu_load_average_1m":             {},
		"cpu_load_average_5m":             {},
		"cpu_load_average_15m":            {},
		"paging_usage_bytes":              {},
		"paging_operations_per_second":    {},
		"pod_phase":                       {},
		"namespace_phase":                 {},
		"node_condition_ready":            {},
		"node_condition_memory_pressure":  {},
		"node_condition_disk_pressure":    {},
		"node_allocatable_cpu_millicores": {},
		"node_allocatable_memory_bytes":   {},
		"deployment_available":            {},
		"daemonset_desired_nodes":         {},
		"daemonset_ready_nodes":           {},
		"statefulset_desired_pods":        {},
		"statefulset_ready_pods":          {},
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
