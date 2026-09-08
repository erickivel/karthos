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
		"container.cpu.usage":         {"k8s.container.cpu.usage", 1000},
		"container.memory.usage":      {"k8s.container.memory.usage", 1},
		"k8s.pod.cpu.usage":           {"k8s.pod.cpu.usage", 1000},
		"k8s.pod.memory.working_set":  {"k8s.pod.memory.working_set", 1},
		"k8s.node.cpu.usage":          {"k8s.node.cpu.usage", 1000},
		"k8s.node.memory.working_set": {"k8s.node.memory.working_set", 1},
		"k8s.container.cpu.usage":     {"k8s.container.cpu.usage", 1000},
		"k8s.container.memory.usage":  {"k8s.container.memory.usage", 1},
		"k8s.volume.available":        {"k8s.volume.available", 1},
		"k8s.volume.capacity":         {"k8s.volume.capacity", 1},
		"k8s.volume.inodes.free":      {"k8s.volume.inodes.free", 1},
		"k8s.volume.inodes.used":      {"k8s.volume.inodes.used", 1},
		"k8s.volume.inodes":           {"k8s.volume.inodes", 1},
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, expected.canonical, parsed.CanonicalName)
		require.Equal(t, expected.factor, parsed.ConversionFactor)
	}
}
