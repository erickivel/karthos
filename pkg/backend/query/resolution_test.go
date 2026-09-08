package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func TestResolveMetricQueryUsesOneHourFifteenSecondTrendDefaults(t *testing.T) {
	now := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)

	got, err := ResolveMetricQuery(MetricQuery{MetricName: "k8s.pod.cpu.usage"}, now)

	require.NoError(t, err)
	assert.Equal(t, now.Add(-time.Hour), got.From)
	assert.Equal(t, now, got.To)
	assert.Equal(t, 15*time.Second, got.Step)
	assert.Equal(t, 5*time.Minute, got.MaxSampleAge)
}

func TestResolveMetricQueryAnchorsTheDefaultWindowAtExplicitEnd(t *testing.T) {
	end := time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC)

	got, err := ResolveMetricQuery(MetricQuery{MetricName: "k8s.pod.cpu.usage", To: end}, end.Add(time.Hour))

	require.NoError(t, err)
	assert.Equal(t, end.Add(-time.Hour), got.From)
	assert.Equal(t, end, got.To)
}

func TestResolveMetricQueryRejectsInvertedRange(t *testing.T) {
	_, err := ResolveMetricQuery(MetricQuery{
		MetricName: "k8s.pod.cpu.usage",
		From:       time.Date(2026, 8, 25, 16, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 8, 25, 15, 0, 0, 0, time.UTC),
	}, time.Now())

	require.ErrorContains(t, err, "from must not be after to")
}

func TestEvaluationPointsIncludeBothRangeBounds(t *testing.T) {
	resolved := ResolvedMetricQuery{
		MetricQuery: MetricQuery{
			MetricName: "k8s.pod.cpu.usage",
			From:       time.Date(2026, 8, 25, 15, 0, 3, 0, time.UTC),
			To:         time.Date(2026, 8, 25, 15, 0, 33, 0, time.UTC),
			Step:       15 * time.Second,
		},
	}

	assert.Equal(t, []time.Time{
		resolved.From,
		resolved.From.Add(15 * time.Second),
		resolved.To,
	}, resolved.EvaluationPoints())
}

func TestFilterEdgesUsesEveryNonEmptyPredicate(t *testing.T) {
	serviceA := entities.ComputeEntityID(entities.KindService, "cluster", "default", "a")
	serviceB := entities.ComputeEntityID(entities.KindService, "cluster", "default", "b")
	node := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	edges := []topology.Edge{
		topology.NewEdge(serviceA, serviceB, topology.RelCalls, time.Now()),
		topology.NewEdge(serviceA, node, topology.RelRunsOn, time.Now()),
	}

	got := FilterEdges(edges, EdgeQuery{Source: serviceA, Target: serviceB, Relation: topology.RelCalls})

	require.Len(t, got, 1)
	assert.Equal(t, topology.RelCalls, got[0].Relation)
}
