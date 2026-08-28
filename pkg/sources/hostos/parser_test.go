package hostos

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/semconv"
)

func TestParseExpandsHostDirectionalMetrics(t *testing.T) {
	for _, tt := range []struct {
		name      string
		raw       string
		direction string
		canonical string
	}{
		{name: "disk read", raw: "system.disk.io", direction: "read", canonical: "system.disk.io.read"},
		{name: "disk write", raw: "system.disk.io", direction: "write", canonical: "system.disk.io.write"},
		{name: "network receive", raw: "system.network.io", direction: "receive", canonical: "system.network.io.receive"},
		{name: "network transmit", raw: "system.network.io", direction: "transmit", canonical: "system.network.io.transmit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parsed, ok := Parse(tt.raw, map[string]string{semconv.AttrDirection: tt.direction})
			require.True(t, ok)
			require.Equal(t, tt.canonical, parsed.CanonicalName)
		})
	}
}

func TestParseRejectsAmbiguousHostDirectionalMetrics(t *testing.T) {
	for _, tt := range []struct {
		name  string
		raw   string
		attrs map[string]string
	}{
		{name: "missing disk direction", raw: "system.disk.io", attrs: map[string]string{}},
		{name: "invalid disk direction", raw: "system.disk.io", attrs: map[string]string{semconv.AttrDirection: "flush"}},
		{name: "missing network direction", raw: "system.network.io", attrs: map[string]string{}},
		{name: "unsupported network shorthand", raw: "system.network.io", attrs: map[string]string{semconv.AttrDirection: "tx"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := Parse(tt.raw, tt.attrs)
			require.False(t, ok)
		})
	}
}
