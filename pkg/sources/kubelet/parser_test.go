package kubelet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesKubeletContainerMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"container.cpu.usage":    "k8s.container.cpu.usage",
		"container.memory.usage": "k8s.container.memory.usage",
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, canonical, parsed.CanonicalName)
	}
}
