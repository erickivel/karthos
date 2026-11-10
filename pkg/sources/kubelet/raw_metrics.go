package kubelet

// rawMetricNames translates kubeletstats wire names into catalog canonical
// names. It is private because only this source parser may use it.
var rawMetricNames = map[string]string{
	"k8s.volume.available":   "k8s.pod.volume.available",
	"k8s.volume.capacity":    "k8s.pod.volume.capacity",
	"k8s.volume.inodes.free": "k8s.pod.volume.inode.free",
	"k8s.volume.inodes.used": "k8s.pod.volume.inode.used",
	"k8s.volume.inodes":      "k8s.pod.volume.inode.count",
}
