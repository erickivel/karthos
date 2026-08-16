package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectRegistryFirstMatchWinsAndFallsBackToIdentity(t *testing.T) {
	registry := NewDialectRegistry(
		DialectFromMap(map[string]string{
			"http_requests_total": "custom.http.count",
		}),
		DialectFromMap(map[string]string{
			"http_requests_total": "http.server.request.count",
		}),
	)

	require.Equal(t, "custom.http.count", registry.Translate("http_requests_total"))
	require.Equal(t, "k8s.pod.cpu.usage", registry.Translate("k8s.pod.cpu.usage"))
}

func TestSourceDialectRegistryUsesExplicitSource(t *testing.T) {
	registry := NewSourceDialectRegistry(map[string]Dialect{
		"source-a": DialectMap{"shared.raw": "canonical.a"},
		"source-b": DialectMap{"shared.raw": "canonical.b"},
	})

	require.Equal(t, "canonical.a", registry.TranslateForSource("source-a", "shared.raw"))
	require.Equal(t, "canonical.b", registry.TranslateForSource("source-b", "shared.raw"))
	require.Equal(t, "shared.raw", registry.TranslateForSource("unknown", "shared.raw"))
}
