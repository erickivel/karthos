package metrics

type Unit string

const (
	UnitMillicores     Unit = "millicores"
	UnitBytes          Unit = "bytes"
	UnitBytesPerSecond Unit = "bytes_per_second"
	UnitRatio          Unit = "ratio"
	UnitPerSecond      Unit = "per_second"
	UnitNanoseconds    Unit = "nanoseconds"
	UnitHertz          Unit = "hertz"
	UnitCount          Unit = "count"
)
