package kubecluster

// rawMetricNames translates k8s_cluster receiver wire names into catalog
// canonical names.
var rawMetricNames = map[string]string{
	"k8s.container.cpu_request":    "k8s.container.cpu.request",
	"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
	"k8s.container.memory_request": "k8s.container.memory.request",
	"k8s.container.memory_limit":   "k8s.container.memory.limit",
	"k8s.container.restarts":       "k8s.container.restart.count",
	"k8s.pod.phase":                "k8s.pod.status.phase",
	"k8s.pod.status_reason":        "k8s.pod.status.reason",
}
