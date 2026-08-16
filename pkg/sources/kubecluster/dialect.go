package kubecluster

import "github.com/erickivel/oksg/pkg/core/metrics"

// Dialect translates k8s_cluster receiver wire names into catalog canonical
// names. Container ready/restarts already match the catalog and need no entry.
var Dialect = metrics.DialectMap{
	"k8s.container.cpu_request":    "k8s.container.cpu.request",
	"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
	"k8s.container.memory_request": "k8s.container.memory.request",
	"k8s.container.memory_limit":   "k8s.container.memory.limit",
}
