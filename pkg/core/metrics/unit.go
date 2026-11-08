package metrics

type Unit string

const (
	UnitMillicores     Unit = "millicores"
	UnitCores          Unit = "cores"
	UnitSeconds        Unit = "seconds"
	UnitRatio          Unit = "ratio"
	UnitBytes          Unit = "bytes"
	UnitBytesPerSecond Unit = "bytes_per_second"
	UnitPercent        Unit = "percent"
	UnitPerSecond      Unit = "per_second"
	UnitNanoseconds    Unit = "nanoseconds"
	UnitCount          Unit = "count"
	UnitResource       Unit = "resource"
)

// IsValidUnit reports whether unit is part of the canonical public unit
// contract. Stored source-provided values must pass this check before they can
// override a catalog fallback.
func IsValidUnit(unit Unit) bool {
	switch unit {
	case UnitMillicores,
		UnitCores,
		UnitSeconds,
		UnitRatio,
		UnitBytes,
		UnitBytesPerSecond,
		UnitPercent,
		UnitPerSecond,
		UnitNanoseconds,
		UnitCount,
		UnitResource:
		return true
	default:
		return false
	}
}
