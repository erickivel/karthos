package hostos

import (
	"context"
	"strings"

	"github.com/erickivel/karthos/pkg/sources/contract"
)

func Normalize(_ context.Context, _ contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, "state", "protocol", "direction")
	switch {
	case strings.HasPrefix(plan.MetricName, "system.cpu."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "cpu")
	case strings.HasPrefix(plan.MetricName, "system.network."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device:network_interface_name", "network.interface.name:network_interface_name")
	case strings.HasPrefix(plan.MetricName, "system.filesystem."):
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device", "mountpoint", "type:filesystem_type", "filesystem.type:filesystem_type", "mode:filesystem_mode")
	default:
		contract.PreserveLabels(plan.Attrs, plan.Labels, "device", "mountpoint")
	}
	return nil
}
