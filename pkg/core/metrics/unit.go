package metrics

type Unit string

const (
	UnitMillicores           Unit = "millicores"
	UnitBytes                Unit = "bytes"
	UnitBytesPerSecond       Unit = "bytes_per_second"
	UnitRatio                Unit = "ratio"
	UnitPercent              Unit = "percent"
	UnitPerSecond            Unit = "per_second"
	UnitNanoseconds          Unit = "nanoseconds"
	UnitNanosecondsPerSecond Unit = "nanoseconds_per_second"
	UnitHertz                Unit = "hertz"
	UnitCount                Unit = "count"
	UnitResource             Unit = "resource"
)

// IsValidUnit reports whether unit is part of the canonical public unit
// contract. Stored source-provided values must pass this check before they can
// override a catalog fallback.
func IsValidUnit(unit Unit) bool {
	switch unit {
	case UnitMillicores,
		UnitBytes,
		UnitBytesPerSecond,
		UnitRatio,
		UnitPercent,
		UnitPerSecond,
		UnitNanoseconds,
		UnitNanosecondsPerSecond,
		UnitHertz,
		UnitCount,
		UnitResource:
		return true
	default:
		return false
	}
}
