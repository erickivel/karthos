package ingest

import (
	"context"
	"strings"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

const (
	// Collector Contrib v0.156.0 uses gopsutil v4.26.5, whose Linux paging
	// parser multiplies count-valued /proc/vmstat fields by 4 KiB. Keep this
	// version marker coupled to the collector pin through a contract test.
	hostPagingScalingCollectorTag = "0.156.0"
	hostPagingCountScale          = 1.0 / 4096.0
)

func hostOSSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceHostOS,
		matches: func(_ string, canonicalName string) bool {
			return strings.HasPrefix(canonicalName, "system.")
		},
		apply: applyHostOSDialect,
	}
}

func applyHostOSDialect(_ context.Context, _ EntityResolver, plan *normalizationPlan) error {
	preserveLabels(plan.attrs, plan.labels, "state", "protocol", "direction")

	switch {
	case strings.HasPrefix(plan.metricName, "system.cpu."):
		preserveLabels(plan.attrs, plan.labels, "cpu")
	case strings.HasPrefix(plan.metricName, "system.network."):
		preserveLabels(plan.attrs, plan.labels,
			"device:network_interface_name",
			"network.interface.name:network_interface_name",
		)
	case strings.HasPrefix(plan.metricName, "system.filesystem."):
		preserveLabels(plan.attrs, plan.labels,
			"device",
			"mountpoint",
			"type:filesystem_type",
			"filesystem.type:filesystem_type",
			"mode:filesystem_mode",
		)
	case strings.HasPrefix(plan.metricName, "system.paging."):
		preserveLabels(plan.attrs, plan.labels, "device", "type:paging_type")
	default:
		preserveLabels(plan.attrs, plan.labels, "device", "mountpoint")
	}

	if plan.metricName == "system.paging.faults" || plan.metricName == "system.paging.operations" {
		plan.conversionFactor *= hostPagingCountScale
	}

	switch plan.metricName {
	case "system.disk.io":
		if kvString(plan.attrs, semconv.AttrDirection) == "write" {
			plan.metricName = "system.disk.io.write"
		} else {
			plan.metricName = "system.disk.io.read"
		}
	case "system.network.io":
		direction := kvString(plan.attrs, semconv.AttrDirection)
		if direction == "transmit" || direction == "tx" {
			plan.metricName = "system.network.io.transmit"
		} else {
			plan.metricName = "system.network.io.receive"
		}
	}
	return nil
}
