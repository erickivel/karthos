package metrics

import (
	"fmt"
	"slices"
	"time"
)

type AggregationMode string

const (
	AggregationLatest     AggregationMode = "latest"
	AggregationWindowRate AggregationMode = "window_rate"
	AggregationActiveRate AggregationMode = "active_rate"
	AggregationIncrease   AggregationMode = "increase"
	// Internal modes describe window-only derived metrics. They are emitted in
	// effective query metadata but cannot be selected via aggregation=.
	AggregationWindowMean  AggregationMode = "window_mean"
	AggregationWindowRatio AggregationMode = "window_ratio"

	MinAggregationWindow     = time.Minute
	MaxAggregationWindow     = time.Hour
	DefaultAggregationWindow = time.Minute
)

var counterAggregationModes = []AggregationMode{
	AggregationLatest,
	AggregationWindowRate,
	AggregationActiveRate,
	AggregationIncrease,
}

// AggregationSpec is the query contract declared by a canonical metric.
type AggregationSpec struct {
	Supported     []AggregationMode `json:"supported"`
	Default       AggregationMode   `json:"default"`
	DefaultWindow time.Duration     `json:"-"`
	Window        string            `json:"default_window,omitempty"`
	BaseUnit      Unit              `json:"base_unit"`
	RateUnit      Unit              `json:"rate_unit"`
}

// ResolvedAggregation is validated query behavior ready for a storage adapter.
type ResolvedAggregation struct {
	Mode   AggregationMode
	Window time.Duration
	Unit   Unit
}

func IsWindowedMeanMetric(name string) bool {
	metric, ok := defaultCatalogMetric(name)
	return ok && metric.Aggregation != nil && metric.Aggregation.Default == AggregationWindowMean
}

func windowRateCounterAggregation(baseUnit, rateUnit Unit) *AggregationSpec {
	return &AggregationSpec{
		Supported:     slices.Clone(counterAggregationModes),
		Default:       AggregationWindowRate,
		DefaultWindow: DefaultAggregationWindow,
		Window:        FormatAggregationWindow(DefaultAggregationWindow),
		BaseUnit:      baseUnit,
		RateUnit:      rateUnit,
	}
}

func latestCounterAggregation(baseUnit, rateUnit Unit) *AggregationSpec {
	return &AggregationSpec{
		Supported:     slices.Clone(counterAggregationModes),
		Default:       AggregationLatest,
		DefaultWindow: DefaultAggregationWindow,
		Window:        FormatAggregationWindow(DefaultAggregationWindow),
		BaseUnit:      baseUnit,
		RateUnit:      rateUnit,
	}
}

func windowDerivedAggregation(mode AggregationMode, unit Unit) *AggregationSpec {
	return &AggregationSpec{
		Default:       mode,
		DefaultWindow: DefaultAggregationWindow,
		Window:        FormatAggregationWindow(DefaultAggregationWindow),
		BaseUnit:      unit,
		RateUnit:      unit,
	}
}

func FormatAggregationWindow(window time.Duration) string {
	if window%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(window/time.Hour))
	}
	if window%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(window/time.Minute))
	}
	return window.String()
}

func ResolveAggregation(metric CanonicalMetric, modeParam, windowParam string) (ResolvedAggregation, error) {
	if metric.Aggregation == nil {
		if modeParam != "" {
			return ResolvedAggregation{}, fmt.Errorf("aggregation is not supported for metric %q", metric.Name)
		}
		if windowParam != "" {
			return ResolvedAggregation{}, fmt.Errorf("window is not supported for metric %q", metric.Name)
		}
		return ResolvedAggregation{Mode: AggregationLatest, Unit: metric.Unit}, nil
	}

	if metric.Aggregation.Default == AggregationWindowMean || metric.Aggregation.Default == AggregationWindowRatio {
		if modeParam != "" {
			return ResolvedAggregation{}, fmt.Errorf("aggregation is not supported for metric %q", metric.Name)
		}
		window, err := resolveWindow(windowParam, metric.Aggregation.DefaultWindow)
		if err != nil {
			return ResolvedAggregation{}, err
		}
		return ResolvedAggregation{Mode: metric.Aggregation.Default, Window: window, Unit: metric.Aggregation.BaseUnit}, nil
	}

	mode := AggregationMode(modeParam)
	if mode == "" {
		mode = metric.Aggregation.Default
	}
	if !slices.Contains(metric.Aggregation.Supported, mode) {
		return ResolvedAggregation{}, fmt.Errorf("aggregation %q is not supported for metric %q", mode, metric.Name)
	}

	if mode == AggregationLatest {
		if windowParam != "" {
			return ResolvedAggregation{}, fmt.Errorf("window is not supported with aggregation %q", mode)
		}
		return ResolvedAggregation{Mode: mode, Unit: metric.Aggregation.BaseUnit}, nil
	}

	window, err := resolveWindow(windowParam, metric.Aggregation.DefaultWindow)
	if err != nil {
		return ResolvedAggregation{}, err
	}

	unit := metric.Aggregation.RateUnit
	if mode == AggregationIncrease {
		unit = metric.Aggregation.BaseUnit
	}
	return ResolvedAggregation{Mode: mode, Window: window, Unit: unit}, nil
}

func resolveWindow(raw string, fallback time.Duration) (time.Duration, error) {
	window := fallback
	if raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid window: expected Go duration: %w", err)
		}
		window = parsed
	}
	if window < MinAggregationWindow || window > MaxAggregationWindow {
		return 0, fmt.Errorf("window must be between %s and %s", MinAggregationWindow, MaxAggregationWindow)
	}
	return window, nil
}
