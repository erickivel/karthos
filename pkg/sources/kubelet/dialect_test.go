package kubelet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectTranslatesKubeletContainerMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"container.cpu.usage":    "k8s.container.cpu.usage",
		"container.memory.usage": "k8s.container.memory.usage",
	} {
		got, ok := Dialect.Translate(raw)
		require.True(t, ok)
		require.Equal(t, canonical, got)
	}
}
