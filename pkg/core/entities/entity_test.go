package entities

import (
	"strings"
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

func TestIsCanonicalID(t *testing.T) {
	valid := ComputeEntityID(KindService, "cluster-a", "payments", "checkout")
	require.True(t, IsCanonicalID(valid))
	require.False(t, IsCanonicalID("not-an-id"))
	require.False(t, IsCanonicalID(EntityID(strings.ToUpper(string(valid)))))
}

func TestCanonicalIDsKeepDistinctKindsAndComponents(t *testing.T) {
	configMap := ComputeEntityID(KindConfigMap, "cluster-a", "default", "shared-name")
	secret := ComputeEntityID(KindSecret, "cluster-a", "default", "shared-name")
	httpEndpoint := ComputeEntityID(KindEndpoint, "10.0.0.10", "80")
	httpsEndpoint := ComputeEntityID(KindEndpoint, "10.0.0.10", "443")

	require.NotEqual(t, configMap, secret)
	require.NotEqual(t, httpEndpoint, httpsEndpoint)
}
