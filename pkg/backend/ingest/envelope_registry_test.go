package ingest_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/erickivel/karthos/pkg/backend/ingest"
	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
	"github.com/stretchr/testify/require"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

type heartbeatRegistry struct {
	persistence.EntityRegistry
	mu       sync.Mutex
	writes   []persistence.StoredEntity
	attempts int
	failNext bool
}

func newHeartbeatRegistry() *heartbeatRegistry {
	return &heartbeatRegistry{EntityRegistry: persistence.NewInMemoryEntityRegistry()}
}
func (r *heartbeatRegistry) UpsertEntity(ctx context.Context, e entities.Entity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	if r.failNext {
		r.failNext = false
		return errors.New("write failed")
	}
	if err := r.EntityRegistry.UpsertEntity(ctx, e); err != nil {
		return err
	}
	r.writes = append(r.writes, persistence.MaterializeEntity(e))
	return nil
}
func envelopeResolver(t *testing.T, r *ingest.OTLPEntityResolver) ingest.EntityResolver {
	t.Helper()
	factory, ok := any(r).(interface{ ForEnvelope() ingest.EntityResolver })
	require.True(t, ok, "OTLP resolver must provide an isolated envelope scope")
	return factory.ForEnvelope()
}
func TestEnvelopeRegistryPreservesMetadataTransitionsAndCopies(t *testing.T) {
	registry := newHeartbeatRegistry()
	base := ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry)
	resolver := envelopeResolver(t, base)
	resource := podResource()
	resource["k8s.pod.uid"] = "A"
	for _, uid := range []string{"A", "A", "B", "B", "A"} {
		resource["k8s.pod.uid"] = uid
		_, err := resolver.Resolve(context.Background(), resource)
		require.NoError(t, err)
	}
	require.Len(t, registry.writes, 3)
	require.Equal(t, "A", registry.writes[0].Metadata["k8s.pod.uid"])
	require.Equal(t, "B", registry.writes[1].Metadata["k8s.pod.uid"])
	require.Equal(t, "A", registry.writes[2].Metadata["k8s.pod.uid"])
	require.Equal(t, registry.writes[0].EntityID, registry.writes[1].EntityID)
	_, err := envelopeResolver(t, base).Resolve(context.Background(), resource)
	require.NoError(t, err)
	require.Len(t, registry.writes, 4, "a later envelope refreshes the ingestion heartbeat")
}
func TestEnvelopeRegistryDoesNotCacheFailedWritesOrIgnoreCancellation(t *testing.T) {
	registry := newHeartbeatRegistry()
	registry.failNext = true
	resolver := envelopeResolver(t, ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry))
	_, err := resolver.Resolve(context.Background(), podResource())
	require.ErrorContains(t, err, "write failed")
	_, err = resolver.Resolve(context.Background(), podResource())
	require.NoError(t, err)
	_, err = resolver.Resolve(context.Background(), podResource())
	require.NoError(t, err)
	require.Equal(t, 2, registry.attempts)
	require.Len(t, registry.writes, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolver.Resolve(ctx, podResource())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 2, registry.attempts)
}
func TestEnvelopeResolverRetainsSourceContractsAndRegistryReads(t *testing.T) {
	registry := newHeartbeatRegistry()
	entity := persistence.StoredEntity{EntityID: entities.ComputeEntityID(entities.KindService, testCluster, "shop", "checkout"), EntityKind: entities.KindService, Cluster: testCluster, Namespace: "shop", Name: "checkout"}
	require.NoError(t, registry.UpsertEntity(context.Background(), entity))
	resolver := envelopeResolver(t, ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry))
	service, ok := resolver.(contract.ServiceResolver)
	require.True(t, ok)
	cluster, ok := resolver.(contract.ClusterResolver)
	require.True(t, ok)
	require.Equal(t, testCluster, cluster.ClusterName())
	id, ok := service.ResolveServiceMetricEntity(context.Background(), nil, "checkout", "")
	require.True(t, ok)
	require.Equal(t, entity.EntityID, id)
	// Reads must remain live: adding an ambiguous candidate changes the answer.
	entity.EntityID = entities.ComputeEntityID(entities.KindService, testCluster, "other", "checkout")
	entity.Namespace = "other"
	require.NoError(t, registry.UpsertEntity(context.Background(), entity))
	_, ok = service.ResolveServiceMetricEntity(context.Background(), nil, "checkout", "")
	require.False(t, ok)
}
func TestNormalizerCoalescesHeartbeatsWithoutChangingSamples(t *testing.T) {
	registry := newHeartbeatRegistry()
	normalizer := ingest.NewOTLPNormalizer(ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry), sources.DefaultRegistry())
	stamp := uint64(time.Date(2026, 11, 13, 0, 0, 0, 123, time.UTC).UnixNano())
	rm := makeGaugeRM("k8s.pod.memory.working_set", 123, stamp)
	gauge := rm.ScopeMetrics[0].Metrics[0].GetGauge()
	gauge.DataPoints = append(gauge.DataPoints, &metricspb.NumberDataPoint{TimeUnixNano: stamp + 1, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 456}})
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), rm)
	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	reference, err := newNormalizer().Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Equal(t, reference, samples)
	require.Len(t, samples, 2)
	require.Len(t, registry.writes, 1)
	_, err = normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, registry.writes, 2)
}
func TestNormalizerEnvelopeScopesAreConcurrentAndIndependent(t *testing.T) {
	registry := newHeartbeatRegistry()
	normalizer := ingest.NewOTLPNormalizer(ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry), sources.DefaultRegistry())
	rm := makeGaugeRM("k8s.pod.memory.working_set", 123, 123456789)
	rm.ScopeMetrics[0].Metrics = append(rm.ScopeMetrics[0].Metrics, rm.ScopeMetrics[0].Metrics[0])
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), rm)
	var wg sync.WaitGroup
	failures := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := normalizer.Normalize(context.Background(), env); failures <- err }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Len(t, registry.writes, 16)
}

func TestEnvelopeResolverCanStartFreshFromAnExistingScope(t *testing.T) {
	registry := newHeartbeatRegistry()
	first := envelopeResolver(t, ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry))
	_, err := first.Resolve(context.Background(), podResource())
	require.NoError(t, err)
	factory, ok := first.(interface{ ForEnvelope() ingest.EntityResolver })
	require.True(t, ok)
	_, err = factory.ForEnvelope().Resolve(context.Background(), podResource())
	require.NoError(t, err)
	require.Len(t, registry.writes, 2, "starting a new scope must not retain a previous scope's suppression")
}
