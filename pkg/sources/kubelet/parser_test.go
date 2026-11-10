package kubelet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesEverySupportedKubeletMetric(t *testing.T) {
	for raw, expected := range map[string]struct {
		canonical string
		factor    float64
	}{
		"container.cpu.usage":         {"container.cpu.usage", 1},
		"container.memory.usage":      {"container.memory.usage", 1},
		"k8s.pod.cpu.usage":           {"k8s.pod.cpu.usage", 1},
		"k8s.pod.memory.working_set":  {"k8s.pod.memory.working_set", 1},
		"k8s.node.cpu.usage":          {"k8s.node.cpu.usage", 1},
		"k8s.node.memory.working_set": {"k8s.node.memory.working_set", 1},
		"k8s.volume.available":        {"k8s.pod.volume.available", 1},
		"k8s.volume.capacity":         {"k8s.pod.volume.capacity", 1},
		"k8s.volume.inodes.free":      {"k8s.pod.volume.inode.free", 1},
		"k8s.volume.inodes.used":      {"k8s.pod.volume.inode.used", 1},
		"k8s.volume.inodes":           {"k8s.pod.volume.inode.count", 1},
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, expected.canonical, parsed.CanonicalName)
		require.Equal(t, expected.factor, parsed.ConversionFactor)
	}
}
