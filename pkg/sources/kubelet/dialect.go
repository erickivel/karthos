package kubelet

import "github.com/erickivel/oksg/pkg/core/metrics"

// Dialect translates kubeletstats wire names into catalog canonical names.
var Dialect = metrics.DialectMap{
	"container.cpu.usage":    "k8s.container.cpu.usage",
	"container.memory.usage": "k8s.container.memory.usage",
}
