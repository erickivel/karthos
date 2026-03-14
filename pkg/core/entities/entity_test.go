package entities

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComputeEntityIDIsDeterministic(t *testing.T) {
	first := ComputeEntityID(KindService, "cluster-a", "payments", "service", "checkout")
	second := ComputeEntityID(KindService, "cluster-a", "payments", "service", "checkout")

	require.Equal(t, first, second)
}

func TestComputeEntityIDIncludesKindInIdentity(t *testing.T) {
	serviceID := ComputeEntityID(KindService, "cluster-a", "payments", "checkout")
	workloadID := ComputeEntityID(KindWorkload, "cluster-a", "payments", "checkout")

	require.NotEqual(t, serviceID, workloadID)
}

func TestClusterImplementsEntityInterface(t *testing.T) {
	cluster := NewCluster("cluster-a", map[string]string{"env": "dev"})

	require.Equal(t, KindCluster, cluster.Kind())
	require.NotEmpty(t, cluster.ID())
}

func TestEndpointIdentityIncludesPort(t *testing.T) {
	httpEndpoint := NewEndpoint("10.0.0.10", 80, nil)
	httpsEndpoint := NewEndpoint("10.0.0.10", 443, nil)

	require.NotEqual(t, httpEndpoint.ID(), httpsEndpoint.ID())
}
