package kubelet

// rawMetricNames translates kubeletstats wire names into catalog canonical
// names. It is private because only this source parser may use it.
var rawMetricNames = map[string]string{
	"container.cpu.usage":    "k8s.container.cpu.usage",
	"container.memory.usage": "k8s.container.memory.usage",
}
