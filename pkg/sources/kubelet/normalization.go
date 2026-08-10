package kubelet

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"strings"
)

func Normalize(_ context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, semconv.AttrDirection, semconv.AttrInterface+":"+semconv.LabelNetworkInterfaceName, semconv.LabelNetworkInterfaceName, semconv.AttrK8sPVCName+":"+semconv.LabelPVCName, "persistentvolumeclaim:"+semconv.LabelPVCName, semconv.LabelPVCName, semconv.AttrK8sVolumeName+":"+semconv.LabelVolumeName, semconv.LabelVolumeName, semconv.AttrK8sVolumeType+":"+semconv.LabelVolumeType, semconv.LabelVolumeType, semconv.ResourceK8sPodName, semconv.ResourceK8sNamespaceName)
	contract.PreserveResourceLabels(plan.Resource, plan.Labels, semconv.AttrK8sPVCName+":"+semconv.LabelPVCName, semconv.AttrK8sVolumeName+":"+semconv.LabelVolumeName, semconv.AttrK8sVolumeType+":"+semconv.LabelVolumeType)
	if !strings.HasPrefix(plan.MetricName, "k8s.volume.") {
		return nil
	}
	namespace, claim := plan.Resource[semconv.ResourceK8sNamespaceName], plan.Resource[semconv.AttrK8sPVCName]
	if claim == "" {
		claim = contract.FirstLabelValue(plan.Attrs, semconv.AttrK8sPVCName, "persistentvolumeclaim", semconv.LabelPVCName)
	}
	if namespace == "" || claim == "" {
		return nil
	}
	cluster := plan.Resource[semconv.ResourceK8sClusterName]
	if named, ok := resolver.(contract.ClusterResolver); ok && named.ClusterName() != "" {
		cluster = named.ClusterName()
	}
	plan.EntityID = entities.ComputeEntityID(entities.KindPVC, cluster, namespace, claim)
	return nil
}
