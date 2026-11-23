// Package collector composes provider-neutral collection requirements from the
// selected static source modules. It never renders collector-specific syntax.
package collector

import (
	"fmt"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

const DefaultNamespace = "karthos-system"

type Composer struct {
	namespace string
	modules   sources.Registry
}

func NewComposer(namespace string) *Composer {
	return NewComposerWithRegistry(namespace, sources.DefaultRegistry())
}

// NewComposerWithRegistry uses the same configured modules as source resolution.
func NewComposerWithRegistry(namespace string, modules sources.Registry) *Composer {
	if namespace == "" {
		namespace = DefaultNamespace
	}
	return &Composer{namespace: namespace, modules: modules}
}

func (c *Composer) Compose(capabilityProfile *profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) (collectorspec.CollectionPlan, error) {
	if capabilityProfile == nil {
		return collectorspec.CollectionPlan{}, fmt.Errorf("cluster profile is required")
	}
	clusterName := capabilityProfile.ClusterName
	if clusterName == "" {
		clusterName = string(capabilityProfile.ClusterID)
	}
	if clusterName == "" {
		clusterName = "default-cluster"
	}
	plan := collectorspec.CollectionPlan{
		ClusterName:    clusterName,
		Namespace:      c.namespace,
		EnabledSignals: []string{"metric"},
		// The gateway always accepts OTLP metrics. Agent-owned signals are
		// separately source-stamped before forwarding; direct producers remain
		// untagged and are rejected by backend source-policy validation.
		Gateway: collectorspec.GatewayRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true}},
	}
	if err := c.modules.AppendRequirements(&plan, capabilityProfile, sourcePlan); err != nil {
		return collectorspec.CollectionPlan{}, fmt.Errorf("append source requirements: %w", err)
	}
	if plan.Agent.OTLP.Traces || plan.Gateway.OTLP.Traces {
		plan.EnabledSignals = append(plan.EnabledSignals, "trace")
	}
	if err := plan.Normalize(); err != nil {
		return collectorspec.CollectionPlan{}, err
	}
	return plan, nil
}
