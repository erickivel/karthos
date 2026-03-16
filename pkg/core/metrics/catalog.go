package metrics

var defaultCatalog = []CanonicalMetric{
	{Name: "cpu_usage_millicores", Unit: UnitMillicores, Category: CategoryResource, Description: "Observed CPU usage per entity."},
	{Name: "memory_usage_bytes", Unit: UnitBytes, Category: CategoryResource, Description: "Observed memory usage per entity."},
	{Name: "disk_read_bytes", Unit: UnitBytes, Category: CategoryResource, Description: "Observed disk read volume per entity."},
	{Name: "disk_write_bytes", Unit: UnitBytes, Category: CategoryResource, Description: "Observed disk write volume per entity."},
	{Name: "cpu_capacity_millicores", Unit: UnitMillicores, Category: CategoryCapacity, Description: "Declared or observed CPU capacity."},
	{Name: "cpu_cores", Unit: UnitCount, Category: CategoryCapacity, Description: "CPU core count in the host profile."},
	{Name: "cpu_clock_hz", Unit: UnitHertz, Category: CategoryCapacity, Description: "CPU clock speed in hertz."},
	{Name: "memory_capacity_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Declared or observed memory capacity."},
	{Name: "disk_capacity_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Declared or observed disk capacity."},
	{Name: "net_in_bandwidth_bps", Unit: UnitBytesPerSecond, Category: CategoryCapacity, Description: "Inbound network bandwidth capacity."},
	{Name: "net_out_bandwidth_bps", Unit: UnitBytesPerSecond, Category: CategoryCapacity, Description: "Outbound network bandwidth capacity."},
	{Name: "request_rate_per_second", Unit: UnitPerSecond, Category: CategoryTraffic, Description: "Request rate observed for a service or edge."},
	{Name: "net_rx_bytes", Unit: UnitBytes, Category: CategoryTraffic, Description: "Received network bytes per entity."},
	{Name: "net_tx_bytes", Unit: UnitBytes, Category: CategoryTraffic, Description: "Transmitted network bytes per entity."},
	{Name: "request_duration_nanoseconds", Unit: UnitNanoseconds, Category: CategoryLatency, Description: "Observed request duration for a service."},
	{Name: "edge_latency_nanoseconds", Unit: UnitNanoseconds, Category: CategoryLatency, Description: "Observed request latency between graph edges."},
	{Name: "error_rate_per_second", Unit: UnitPerSecond, Category: CategoryError, Description: "Observed error rate for a service or edge."},
	{Name: "error_ratio", Unit: UnitRatio, Category: CategoryError, Description: "Observed error ratio for a service or edge."},
	{Name: "cpu_request_millicores", Unit: UnitMillicores, Category: CategoryCapacity, Description: "Requested CPU for a container."},
	{Name: "cpu_limit_millicores", Unit: UnitMillicores, Category: CategoryCapacity, Description: "CPU limit for a container."},
	{Name: "memory_request_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Requested memory for a container."},
	{Name: "memory_limit_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Memory limit for a container."},
	{Name: "replica_count", Unit: UnitCount, Category: CategoryCapacity, Description: "Desired replica count for a workload."},
	{Name: "disk_read_bps", Unit: UnitBytesPerSecond, Category: CategoryResource, Description: "Disk read throughput per entity."},
	{Name: "disk_write_bps", Unit: UnitBytesPerSecond, Category: CategoryResource, Description: "Disk write throughput per entity."},
	{Name: "net_rx_bps", Unit: UnitBytesPerSecond, Category: CategoryResource, Description: "Network receive throughput per entity."},
	{Name: "net_tx_bps", Unit: UnitBytesPerSecond, Category: CategoryResource, Description: "Network transmit throughput per entity."},
}

func DefaultCatalog() []CanonicalMetric {
	catalog := make([]CanonicalMetric, len(defaultCatalog))
	copy(catalog, defaultCatalog)
	return catalog
}
