package ingest

import (
	"context"
	"strings"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func kubeletSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceKubelet,
		matches: func(_ string, canonicalName string) bool {
			return strings.HasPrefix(canonicalName, "k8s.volume.") ||
				strings.HasPrefix(canonicalName, "k8s.pod.network.") ||
				strings.HasPrefix(canonicalName, "k8s.node.network.")
		},
		apply: applyKubeletDialect,
	}
}

func applyKubeletDialect(_ context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	preserveLabels(plan.attrs, plan.labels,
		labelAlias(semconv.AttrDirection),
		labelAlias(semconv.AttrInterface+":"+semconv.LabelNetworkInterfaceName),
		labelAlias(semconv.LabelNetworkInterfaceName),
		labelAlias(semconv.AttrK8sPVCName+":"+semconv.LabelPVCName),
		labelAlias("persistentvolumeclaim:"+semconv.LabelPVCName),
		labelAlias(semconv.LabelPVCName),
		labelAlias(semconv.AttrK8sVolumeName+":"+semconv.LabelVolumeName),
		labelAlias(semconv.LabelVolumeName),
		labelAlias(semconv.AttrK8sVolumeType+":"+semconv.LabelVolumeType),
		labelAlias(semconv.LabelVolumeType),
		semconv.ResourceK8sPodName,
		semconv.ResourceK8sNamespaceName,
	)
	preserveResourceLabels(plan.resource, plan.labels,
		labelAlias(semconv.AttrK8sPVCName+":"+semconv.LabelPVCName),
		labelAlias(semconv.AttrK8sVolumeName+":"+semconv.LabelVolumeName),
		labelAlias(semconv.AttrK8sVolumeType+":"+semconv.LabelVolumeType),
	)
	if !strings.HasPrefix(plan.metricName, "k8s.volume.") {
		return nil
	}

	namespace := plan.resource[semconv.ResourceK8sNamespaceName]
	claimName := plan.resource[semconv.AttrK8sPVCName]
	if claimName == "" {
		claimName = firstKVString(plan.attrs, semconv.AttrK8sPVCName, "persistentvolumeclaim", semconv.LabelPVCName)
	}
	if namespace == "" || claimName == "" {
		return nil
	}
	clusterName := plan.resource[semconv.ResourceK8sClusterName]
	if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok && otlpResolver.clusterName != "" {
		clusterName = otlpResolver.clusterName
	}
	plan.entityID = entities.ComputeEntityID(entities.KindPVC, clusterName, namespace, claimName)
	return nil
}
