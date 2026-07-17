package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/collector/configgen"
)

func TestHostPagingScaleMatchesPinnedCollector(t *testing.T) {
	require.Equal(t, hostPagingScalingCollectorTag, configgen.DefaultCollectorTag,
		"re-verify the host paging scale against the new collector dependency before changing the pin")
}
