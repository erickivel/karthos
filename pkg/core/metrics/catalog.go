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

	// Volume metrics (kubeletstats volume group)
	{Name: "volume_available_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Available bytes on a persistent volume."},
	{Name: "volume_capacity_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Total capacity bytes of a persistent volume."},
	{Name: "volume_inodes_free", Unit: UnitCount, Category: CategoryCapacity, Description: "Free inodes on a persistent volume."},
	{Name: "volume_inodes_used", Unit: UnitCount, Category: CategoryResource, Description: "Used inodes on a persistent volume."},

	// Filesystem metrics (hostmetrics filesystem scraper)
	{Name: "filesystem_usage_bytes", Unit: UnitBytes, Category: CategoryResource, Description: "Filesystem usage in bytes per mount point."},
	{Name: "filesystem_utilization", Unit: UnitRatio, Category: CategoryResource, Description: "Filesystem utilization ratio per mount point."},

	// Load average metrics (hostmetrics load scraper)
	{Name: "cpu_load_average_1m", Unit: UnitCount, Category: CategoryResource, Description: "1-minute CPU load average."},
	{Name: "cpu_load_average_5m", Unit: UnitCount, Category: CategoryResource, Description: "5-minute CPU load average."},
	{Name: "cpu_load_average_15m", Unit: UnitCount, Category: CategoryResource, Description: "15-minute CPU load average."},

	// Paging metrics (hostmetrics paging scraper)
	{Name: "paging_usage_bytes", Unit: UnitBytes, Category: CategoryResource, Description: "Paging/swap usage in bytes."},
	{Name: "paging_operations_per_second", Unit: UnitPerSecond, Category: CategoryResource, Description: "Paging operations per second."},

	// Kubernetes state metrics (k8s_cluster receiver)
	{Name: "pod_phase", Unit: UnitCount, Category: CategoryHealth, Description: "Pod phase as a gauge (1=Running, 2=Pending, 3=Succeeded, 4=Failed, 5=Unknown)."},
	{Name: "namespace_phase", Unit: UnitCount, Category: CategoryHealth, Description: "Namespace phase (1=active, 0=terminating)."},
	{Name: "node_condition_ready", Unit: UnitCount, Category: CategoryHealth, Description: "Node Ready condition (1=true, 0=false)."},
	{Name: "node_condition_memory_pressure", Unit: UnitCount, Category: CategoryHealth, Description: "Node MemoryPressure condition (1=true, 0=false)."},
	{Name: "node_condition_disk_pressure", Unit: UnitCount, Category: CategoryHealth, Description: "Node DiskPressure condition (1=true, 0=false)."},
	{Name: "node_allocatable_cpu_millicores", Unit: UnitMillicores, Category: CategoryCapacity, Description: "Node allocatable CPU in millicores."},
	{Name: "node_allocatable_memory_bytes", Unit: UnitBytes, Category: CategoryCapacity, Description: "Node allocatable memory in bytes."},

	// Workload state metrics (k8s_cluster receiver)
	{Name: "deployment_available", Unit: UnitCount, Category: CategoryHealth, Description: "Available replicas for a deployment."},
	{Name: "daemonset_desired_nodes", Unit: UnitCount, Category: CategoryCapacity, Description: "Nodes that should run the daemon pod."},
	{Name: "daemonset_ready_nodes", Unit: UnitCount, Category: CategoryHealth, Description: "Nodes with ready daemon pod instances."},
	{Name: "statefulset_desired_pods", Unit: UnitCount, Category: CategoryCapacity, Description: "Desired pod replicas for a statefulset."},
	{Name: "statefulset_ready_pods", Unit: UnitCount, Category: CategoryHealth, Description: "Ready pods for a statefulset."},
}

func DefaultCatalog() []CanonicalMetric {
	catalog := make([]CanonicalMetric, len(defaultCatalog))
	copy(catalog, defaultCatalog)
	return catalog
}
