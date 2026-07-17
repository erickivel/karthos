package metrics

import "testing"

import "github.com/stretchr/testify/require"

func TestIsValidUnit(t *testing.T) {
	for _, unit := range []Unit{
		UnitMillicores,
		UnitBytes,
		UnitBytesPerSecond,
		UnitRatio,
		UnitPercent,
		UnitPerSecond,
		UnitNanoseconds,
		UnitNanosecondsPerSecond,
		UnitHertz,
		UnitCount,
		UnitResource,
	} {
		require.True(t, IsValidUnit(unit), unit)
	}
	require.False(t, IsValidUnit(""))
	require.False(t, IsValidUnit("cores"))
}
