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

func TestClusterImplementsEntityInterface(t *testing.T) {
	cluster := NewCluster("cluster-a", map[string]string{"env": "dev"})

	require.Equal(t, KindCluster, cluster.Kind())
	require.NotEmpty(t, cluster.ID())
}

func TestConfigMapImplementsEntityInterface(t *testing.T) {
	cm := NewConfigMap("cluster-a", "default", "app-config", nil)

	require.Equal(t, KindConfigMap, cm.Kind())
	require.NotEmpty(t, cm.ID())
}

func TestSecretImplementsEntityInterface(t *testing.T) {
	s := NewSecret("cluster-a", "default", "app-secret", nil)

	require.Equal(t, KindSecret, s.Kind())
	require.NotEmpty(t, s.ID())
}

func TestPVCImplementsEntityInterface(t *testing.T) {
	p := NewPVC("cluster-a", "default", "data-volume", nil)

	require.Equal(t, KindPVC, p.Kind())
	require.NotEmpty(t, p.ID())
}

func TestConfigMapAndSecretHaveDistinctIDs(t *testing.T) {
	cm := NewConfigMap("cluster-a", "default", "shared-name", nil)
	s := NewSecret("cluster-a", "default", "shared-name", nil)

	require.NotEqual(t, cm.ID(), s.ID())
}

func TestEndpointIdentityIncludesPort(t *testing.T) {
	httpEndpoint := NewEndpoint("10.0.0.10", 80, nil)
	httpsEndpoint := NewEndpoint("10.0.0.10", 443, nil)

	require.NotEqual(t, httpEndpoint.ID(), httpsEndpoint.ID())
}
