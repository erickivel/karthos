package hostos

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"strings"
)

const PagingCountScale = 1.0 / 4096.0

func Normalize(_ context.Context, _ contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, "state", "protocol", "direction")
	switch {
	case strings.HasPrefix(plan.MetricName, "system.cpu."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "cpu")
	case strings.HasPrefix(plan.MetricName, "system.network."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device:network_interface_name", "network.interface.name:network_interface_name")
	case strings.HasPrefix(plan.MetricName, "system.filesystem."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device", "mountpoint", "type:filesystem_type", "filesystem.type:filesystem_type", "mode:filesystem_mode")
	case strings.HasPrefix(plan.MetricName, "system.paging."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device", "type:paging_type")
	default:
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device", "mountpoint")
	}
	if plan.MetricName == "system.paging.faults" || plan.MetricName == "system.paging.operations" {
		plan.ConversionFactor *= PagingCountScale
	}
	if plan.MetricName == "system.disk.io" {
		if contract.LabelValue(plan.Attrs, semconv.AttrDirection) == "write" {
			plan.MetricName = "system.disk.io.write"
		} else {
			plan.MetricName = "system.disk.io.read"
		}
	}
	if plan.MetricName == "system.network.io" {
		direction := contract.LabelValue(plan.Attrs, semconv.AttrDirection)
		if direction == "transmit" || direction == "tx" {
			plan.MetricName = "system.network.io.transmit"
		} else {
			plan.MetricName = "system.network.io.receive"
		}
	}
	return nil
}
