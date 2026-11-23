package metrics

import "testing"

import "github.com/stretchr/testify/require"

func TestIsValidUnit(t *testing.T) {
	for _, unit := range []Unit{
		UnitMillicores,
		UnitCores,
		UnitSeconds,
		UnitRatio,
		UnitBytes,
		UnitBytesPerSecond,
		UnitPercent,
		UnitPerSecond,
		UnitNanoseconds,
		UnitCount,
		UnitResource,
	} {
		require.True(t, IsValidUnit(unit), unit)
	}
	require.False(t, IsValidUnit(""))
	require.False(t, IsValidUnit("unsupported"))
}
