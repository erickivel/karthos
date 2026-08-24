package kubernetes

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestPodLifecycleTrackerRejectsDeletedUIDAndAcceptsReplacement(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client)
	require.NoError(t, tracker.Start(ctx))
	require.True(t, tracker.AllowsRunsOn("default", "api-0", "uid-old", "node-a"))
	require.False(t, tracker.AllowsRunsOn("default", "api-0", "uid-old", "node-b"))

	watcher.Delete(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	require.Eventually(t, func() bool {
		return !tracker.AllowsRunsOn("default", "api-0", "uid-old", "node-a")
	}, time.Second, 10*time.Millisecond)

	watcher.Add(podForLifecycle("default", "api-0", "uid-new", "node-a"))
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "api-0", "uid-new", "node-a")
	}, time.Second, 10*time.Millisecond)
	require.False(t, tracker.AllowsRunsOn("default", "api-0", "uid-old", "node-a"))
}

func TestPodLifecycleTrackerRetiresDeletedActiveUIDImmediately(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	retirements := make(chan PodRunsOnRetirement, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client, WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
		retirements <- retirement
		return nil
	}))
	require.NoError(t, tracker.Start(ctx))

	watcher.Delete(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	select {
	case retirement := <-retirements:
		require.Equal(t, "uid-old", retirement.PodUID)
	case <-time.After(time.Second):
		t.Fatal("expected direct deletion retirement")
	}
}

func TestPodLifecycleTrackerKeepsTerminalGraceWhenDeletedBeforeDeadline(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-terminal", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	retirements := make(chan PodRunsOnRetirement, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		withPodLifecycleGraceDuration(50*time.Millisecond),
		WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
			retirements <- retirement
			return nil
		}),
	)
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-terminal", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	require.Eventually(t, func() bool {
		tracker.mu.RLock()
		defer tracker.mu.RUnlock()
		return len(tracker.retirements) == 1
	}, time.Second, 5*time.Millisecond)
	watcher.Delete(terminal)

	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a")
	}, time.Second, 5*time.Millisecond)
	select {
	case retirement := <-retirements:
		t.Fatalf("terminal Pod retired before its grace deadline: %#v", retirement)
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case retirement := <-retirements:
		require.Equal(t, "uid-terminal", retirement.PodUID)
	case <-time.After(time.Second):
		t.Fatal("expected terminal Pod retirement at grace deadline")
	}
}

func TestPodLifecycleTrackerRetiresReplacedActiveUIDImmediately(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	retirements := make(chan PodRunsOnRetirement, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client, WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
		retirements <- retirement
		return nil
	}))
	require.NoError(t, tracker.Start(ctx))

	watcher.Add(podForLifecycle("default", "api-0", "uid-new", "node-a"))
	select {
	case retirement := <-retirements:
		require.Equal(t, "uid-old", retirement.PodUID)
	case <-time.After(time.Second):
		t.Fatal("expected direct replacement retirement")
	}
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "api-0", "uid-new", "node-a")
	}, time.Second, 5*time.Millisecond)
}

func TestPodLifecycleTrackerRetriesRetirementObserverWithoutBlockingWatch(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	var attempts atomic.Int32
	succeeded := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		withPodRetirementRetry(time.Millisecond),
		WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
			if attempts.Add(1) == 1 {
				return errors.New("temporary storage failure")
			}
			succeeded <- struct{}{}
			return nil
		}),
	)
	require.NoError(t, tracker.Start(ctx))

	watcher.Delete(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	watcher.Add(podForLifecycle("default", "api-0", "uid-new", "node-a"))
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "api-0", "uid-new", "node-a")
	}, time.Second, 5*time.Millisecond)
	select {
	case <-succeeded:
		require.EqualValues(t, 2, attempts.Load())
	case <-time.After(time.Second):
		t.Fatal("expected retry after transient retirement failure")
	}
}

func TestPodLifecycleTrackerRetriesRetirementObserverPastThreeFailures(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	var attempts atomic.Int32
	succeeded := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		withPodRetirementRetry(time.Millisecond),
		WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
			if attempts.Add(1) <= 3 {
				return errors.New("temporary storage failure")
			}
			succeeded <- struct{}{}
			return nil
		}),
	)
	require.NoError(t, tracker.Start(ctx))
	watcher.Delete(podForLifecycle("default", "api-0", "uid-old", "node-a"))

	select {
	case <-succeeded:
		require.EqualValues(t, 4, attempts.Load())
	case <-time.After(time.Second):
		t.Fatal("expected retirement retry beyond three failures")
	}
}

func TestPodLifecycleTrackerInitialReconciliationRetiresPreseededReplacement(t *testing.T) {
	store := persistence.NewInMemoryGraphStore()
	podID := entities.ComputeEntityID(entities.KindPod, "cluster", "default", "api-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, "cluster", "node-a")
	old := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	old.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-old")
	old.Metadata = map[string]string{"k8s.pod.uid": "uid-old", "k8s.cluster.name": "cluster"}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{old}}}))

	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-new", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client, WithPodRunsOnReconciliationObserver(func(ctx context.Context, observed []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error) {
		assignments := make([]persistence.RunsOnAssignment, 0, len(observed))
		for _, assignment := range observed {
			assignments = append(assignments, persistence.RunsOnAssignment{
				PodID:  entities.ComputeEntityID(entities.KindPod, "cluster", assignment.Namespace, assignment.PodName),
				NodeID: entities.ComputeEntityID(entities.KindNode, "cluster", assignment.NodeName),
				PodUID: assignment.PodUID,
			})
		}
		_, err := store.ReconcileRunsOn(ctx, "cluster", assignments, time.Now().UTC())
		return nil, err
	}))
	require.NoError(t, tracker.Start(ctx))

	latest, err := store.GetLatest(ctx)
	require.NoError(t, err)
	require.Empty(t, latest.Graph.Edges)
}

func TestPodLifecycleTrackerRestoresPersistedTerminalGraceAfterReplacement(t *testing.T) {
	const cluster = "cluster"
	store := persistence.NewInMemoryGraphStore()
	podID := entities.ComputeEntityID(entities.KindPod, cluster, "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, cluster, "node-a")
	old := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	old.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-old")
	old.Metadata = map[string]string{"k8s.pod.uid": "uid-old", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{old}}}))

	firstWatcher := watch.NewRaceFreeFake()
	firstClient := fake.NewClientset(podForLifecycle("default", "job-0", "uid-old", "node-a"))
	firstClient.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, firstWatcher, nil
	})
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := NewPodLifecycleTracker(firstClient,
		withPodLifecycleGraceDuration(150*time.Millisecond),
		WithPodRunsOnTerminalObserver(func(ctx context.Context, grace PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
			state, err := store.MarkRunsOnTerminal(ctx, persistence.RunsOnTerminalGrace{
				ClusterName: cluster,
				Namespace:   grace.Namespace,
				PodName:     grace.PodName,
				PodUID:      grace.PodUID,
				NodeName:    grace.NodeName,
				PodID:       podID,
				NodeID:      nodeID,
				ObservedAt:  grace.ObservedAt,
				Deadline:    grace.Deadline,
				Confirmed:   grace.Confirmed,
			})
			return PodRunsOnTerminalState(state), err
		}),
		WithPodRunsOnRetirementObserver(func(ctx context.Context, retirement PodRunsOnRetirement) error {
			return store.RetireRunsOn(ctx, podID, nodeID, retirement.PodUID, retirement.RetiredAt)
		}),
	)
	require.NoError(t, first.Start(firstCtx))
	terminal := podForLifecycle("default", "job-0", "uid-old", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	firstWatcher.Modify(terminal)
	firstWatcher.Delete(terminal)
	require.Eventually(t, func() bool {
		edges, err := store.QueryEdges(context.Background(), query.EdgeQuery{Source: podID})
		return err == nil && len(edges) == 1 && edges[0].Metadata["terminal_deadline"] != ""
	}, time.Second, 5*time.Millisecond)
	cancelFirst()

	newAssignment := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	newAssignment.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-new")
	newAssignment.Metadata = map[string]string{"k8s.pod.uid": "uid-new", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{newAssignment}}}))

	secondWatcher := watch.NewRaceFreeFake()
	secondClient := fake.NewClientset(podForLifecycle("default", "job-0", "uid-new", "node-a"))
	secondClient.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, secondWatcher, nil
	})
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	second := NewPodLifecycleTracker(secondClient,
		WithPodRunsOnReconciliationObserver(func(ctx context.Context, observed []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error) {
			active := make([]persistence.RunsOnAssignment, 0, len(observed))
			for _, assignment := range observed {
				active = append(active, persistence.RunsOnAssignment{PodID: podID, NodeID: nodeID, PodUID: assignment.PodUID})
			}
			persisted, err := store.ReconcileRunsOn(ctx, cluster, active, time.Now().UTC())
			grace := make([]PodRunsOnTerminalGrace, 0, len(persisted))
			for _, item := range persisted {
				grace = append(grace, PodRunsOnTerminalGrace{Namespace: item.Namespace, PodName: item.PodName, PodUID: item.PodUID, NodeName: item.NodeName, ObservedAt: item.ObservedAt, Deadline: item.Deadline, Confirmed: item.Confirmed})
			}
			return grace, err
		}),
		WithPodRunsOnRetirementObserver(func(ctx context.Context, retirement PodRunsOnRetirement) error {
			return store.RetireRunsOn(ctx, podID, nodeID, retirement.PodUID, retirement.RetiredAt)
		}),
	)
	require.NoError(t, second.Start(secondCtx))
	require.True(t, second.AllowsRunsOn("default", "job-0", "uid-old", "node-a"))

	latest, err := store.GetLatest(secondCtx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 2, "old terminal UID remains active during its persisted grace")
	require.Eventually(t, func() bool {
		current, err := store.GetLatest(context.Background())
		return err == nil && len(current.Graph.Edges) == 1 && current.Graph.Edges[0].ID == newAssignment.ID
	}, time.Second, 5*time.Millisecond)
}

func TestPodLifecycleTrackerDeniesTerminalRefreshUntilDeadlineMarkerPersists(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-terminal", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	markerStarted := make(chan struct{})
	allowMarker := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client, WithPodRunsOnTerminalObserver(func(_ context.Context, _ PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
		close(markerStarted)
		<-allowMarker
		return PodRunsOnTerminalPending, nil
	}))
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-terminal", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	select {
	case <-markerStarted:
	case <-time.After(time.Second):
		t.Fatal("expected synchronous terminal deadline persistence")
	}
	require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a"))
	close(allowMarker)
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a")
	}, time.Second, 5*time.Millisecond)
}

func TestPodLifecycleTrackerFailsClosedAndRetriesTerminalMarkerWithOriginalDeadline(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-terminal", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	var attempts atomic.Int32
	deadlines := make(chan time.Time, 2)
	retryStarted := make(chan struct{})
	allowRetry := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		withPodLifecycleGraceDuration(200*time.Millisecond),
		withPodRetirementRetry(time.Millisecond),
		WithPodRunsOnTerminalObserver(func(_ context.Context, grace PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
			deadlines <- grace.Deadline
			if attempts.Add(1) == 1 {
				return PodRunsOnTerminalPending, errors.New("initial marker write failed")
			}
			close(retryStarted)
			<-allowRetry
			return PodRunsOnTerminalPending, nil
		}),
	)
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-terminal", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("expected asynchronous terminal marker retry")
	}
	require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a"))
	firstDeadline := <-deadlines
	secondDeadline := <-deadlines
	require.Equal(t, firstDeadline, secondDeadline)
	close(allowRetry)
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a")
	}, time.Second, 5*time.Millisecond)
}

func TestPodLifecycleTrackerRestoresPersistedDeadlineForTerminalUIDStillListed(t *testing.T) {
	const cluster = "cluster"
	store := persistence.NewInMemoryGraphStore()
	podID := entities.ComputeEntityID(entities.KindPod, cluster, "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, cluster, "node-a")
	observedAt := time.Now().UTC()
	deadline := observedAt.Add(200 * time.Millisecond)
	old := topology.NewEdge(podID, nodeID, topology.RelRunsOn, observedAt)
	old.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-terminal")
	old.Metadata = map[string]string{"k8s.pod.uid": "uid-terminal", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{old}}}))
	_, err := store.MarkRunsOnTerminal(context.Background(), persistence.RunsOnTerminalGrace{
		ClusterName: cluster, Namespace: "default", PodName: "job-0", PodUID: "uid-terminal", NodeName: "node-a",
		PodID: podID, NodeID: nodeID, ObservedAt: observedAt, Deadline: deadline, Confirmed: true,
	})
	require.NoError(t, err)

	watcher := watch.NewRaceFreeFake()
	terminal := podForLifecycle("default", "job-0", "uid-terminal", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	client := fake.NewClientset(terminal)
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	var markerWrites atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		WithPodRunsOnTerminalObserver(func(_ context.Context, _ PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
			markerWrites.Add(1)
			return PodRunsOnTerminalPending, nil
		}),
		WithPodRunsOnReconciliationObserver(func(ctx context.Context, observed []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error) {
			active := []persistence.RunsOnAssignment{{PodID: podID, NodeID: nodeID, PodUID: observed[0].PodUID}}
			persisted, err := store.ReconcileRunsOn(ctx, cluster, active, time.Now().UTC())
			grace := make([]PodRunsOnTerminalGrace, 0, len(persisted))
			for _, item := range persisted {
				grace = append(grace, PodRunsOnTerminalGrace{Namespace: item.Namespace, PodName: item.PodName, PodUID: item.PodUID, NodeName: item.NodeName, ObservedAt: item.ObservedAt, Deadline: item.Deadline, Confirmed: item.Confirmed})
			}
			return grace, err
		}),
	)
	require.NoError(t, tracker.Start(ctx))
	require.True(t, tracker.AllowsRunsOn("default", "job-0", "uid-terminal", "node-a"))
	require.Never(t, func() bool {
		return markerWrites.Load() != 0
	}, 30*time.Millisecond, 5*time.Millisecond, "restart must not reset an existing terminal deadline")
	edges, err := store.QueryEdges(ctx, query.EdgeQuery{Source: podID})
	require.NoError(t, err)
	require.Equal(t, deadline.Format(time.RFC3339Nano), edges[0].Metadata["terminal_deadline"])
}

func TestPodLifecycleTrackerRetirementWinsWhenDelayedMarkerRetriesAcrossRestart(t *testing.T) {
	const cluster = "cluster"
	store := persistence.NewInMemoryGraphStore()
	podID := entities.ComputeEntityID(entities.KindPod, cluster, "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, cluster, "node-a")
	old := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	old.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-old")
	old.Metadata = map[string]string{"k8s.pod.uid": "uid-old", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{old}}}))

	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	var markerAttempts atomic.Int32
	retryStarted := make(chan struct{})
	allowRetry := make(chan struct{})
	retiredDurably := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	tracker := NewPodLifecycleTracker(client,
		withPodLifecycleGraceDuration(time.Second),
		withPodRetirementRetry(time.Millisecond),
		WithPodRunsOnTerminalObserver(func(ctx context.Context, grace PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
			if markerAttempts.Add(1) == 1 {
				return PodRunsOnTerminalPending, errors.New("initial marker write failed")
			}
			close(retryStarted)
			<-allowRetry
			state, err := store.MarkRunsOnTerminal(ctx, persistence.RunsOnTerminalGrace{
				ClusterName: cluster, Namespace: grace.Namespace, PodName: grace.PodName, PodUID: grace.PodUID, NodeName: grace.NodeName,
				PodID: podID, NodeID: nodeID, ObservedAt: grace.ObservedAt, Deadline: grace.Deadline, Confirmed: grace.Confirmed,
			})
			return PodRunsOnTerminalState(state), err
		}),
		WithPodRunsOnRetirementObserver(func(retirementCtx context.Context, retirement PodRunsOnRetirement) error {
			if err := store.RetireRunsOn(retirementCtx, podID, nodeID, retirement.PodUID, retirement.RetiredAt); err != nil {
				return err
			}
			close(retiredDurably)
			<-retirementCtx.Done() // simulate crash before callback completion
			return retirementCtx.Err()
		}),
	)
	require.NoError(t, tracker.Start(ctx))
	terminal := podForLifecycle("default", "job-0", "uid-old", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("expected delayed terminal marker retry")
	}
	watcher.Delete(terminal)
	select {
	case <-retiredDurably:
	case <-time.After(time.Second):
		t.Fatal("expected atomic direct retirement before simulated crash")
	}

	newAssignment := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	newAssignment.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-new")
	newAssignment.Metadata = map[string]string{"k8s.pod.uid": "uid-new", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{newAssignment}}}))
	watcher.Add(podForLifecycle("default", "job-0", "uid-new", "node-a"))
	close(allowRetry)
	require.Eventually(t, func() bool {
		return markerAttempts.Load() == 2 && !tracker.AllowsRunsOn("default", "job-0", "uid-old", "node-a")
	}, time.Second, 5*time.Millisecond)
	cancel() // crash before the retirement callback returns

	restartWatcher := watch.NewRaceFreeFake()
	restartClient := fake.NewClientset(podForLifecycle("default", "job-0", "uid-new", "node-a"))
	restartClient.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, restartWatcher, nil
	})
	restartCtx, cancelRestart := context.WithCancel(context.Background())
	defer cancelRestart()
	restarted := NewPodLifecycleTracker(restartClient, WithPodRunsOnReconciliationObserver(func(ctx context.Context, observed []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error) {
		active := []persistence.RunsOnAssignment{{PodID: podID, NodeID: nodeID, PodUID: observed[0].PodUID}}
		persisted, err := store.ReconcileRunsOn(ctx, cluster, active, time.Now().UTC())
		grace := make([]PodRunsOnTerminalGrace, 0, len(persisted))
		for _, item := range persisted {
			grace = append(grace, PodRunsOnTerminalGrace{Namespace: item.Namespace, PodName: item.PodName, PodUID: item.PodUID, NodeName: item.NodeName, Deadline: item.Deadline, Confirmed: item.Confirmed})
		}
		return grace, err
	}))
	require.NoError(t, restarted.Start(restartCtx))
	latest, err := store.GetLatest(restartCtx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, newAssignment.ID, latest.Graph.Edges[0].ID)
}

func TestPodLifecycleTrackerRestartRetiresUnconfirmedRetryMarkerAfterReplacement(t *testing.T) {
	const cluster = "cluster"
	store := persistence.NewInMemoryGraphStore()
	podID := entities.ComputeEntityID(entities.KindPod, cluster, "default", "job-0")
	nodeID := entities.ComputeEntityID(entities.KindNode, cluster, "node-a")
	old := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	old.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-old")
	old.Metadata = map[string]string{"k8s.pod.uid": "uid-old", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{old}}}))

	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})
	var markerAttempts atomic.Int32
	retryStarted := make(chan struct{})
	allowRetryWrite := make(chan struct{})
	retirementAttempted := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	tracker := NewPodLifecycleTracker(client,
		withPodLifecycleGraceDuration(time.Second),
		withPodRetirementRetry(time.Millisecond),
		WithPodRunsOnTerminalObserver(func(markerCtx context.Context, grace PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
			if markerAttempts.Add(1) == 1 {
				return PodRunsOnTerminalPending, errors.New("initial marker write failed")
			}
			close(retryStarted)
			<-allowRetryWrite
			state, err := store.MarkRunsOnTerminal(markerCtx, persistence.RunsOnTerminalGrace{
				ClusterName: cluster, Namespace: grace.Namespace, PodName: grace.PodName, PodUID: grace.PodUID, NodeName: grace.NodeName,
				PodID: podID, NodeID: nodeID, ObservedAt: grace.ObservedAt, Deadline: grace.Deadline, Confirmed: grace.Confirmed,
			})
			return PodRunsOnTerminalState(state), err
		}),
		WithPodRunsOnRetirementObserver(func(context.Context, PodRunsOnRetirement) error {
			select {
			case <-retirementAttempted:
			default:
				close(retirementAttempted)
			}
			return errors.New("simulate crash before retirement persistence")
		}),
	)
	require.NoError(t, tracker.Start(ctx))
	terminal := podForLifecycle("default", "job-0", "uid-old", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("expected delayed terminal marker retry")
	}
	watcher.Delete(terminal)
	select {
	case <-retirementAttempted:
	case <-time.After(time.Second):
		t.Fatal("expected retirement attempt before simulated crash")
	}

	newAssignment := topology.NewEdge(podID, nodeID, topology.RelRunsOn, time.Now().UTC())
	newAssignment.ID = topology.ComputeEdgeIDWithDiscriminator(podID, nodeID, topology.RelRunsOn, "uid-new")
	newAssignment.Metadata = map[string]string{"k8s.pod.uid": "uid-new", "k8s.cluster.name": cluster}
	require.NoError(t, store.PutObservedSnapshot(context.Background(), topology.Snapshot{Graph: topology.Graph{Edges: []topology.Edge{newAssignment}}}))
	close(allowRetryWrite)
	require.Eventually(t, func() bool {
		edges, err := store.QueryEdges(context.Background(), query.EdgeQuery{Source: podID})
		if err != nil || markerAttempts.Load() != 2 {
			return false
		}
		for _, edge := range edges {
			if edge.ID == old.ID {
				return edge.Metadata["terminal_deadline"] != "" && edge.Metadata["terminal_grace_confirmed"] == ""
			}
		}
		return false
	}, time.Second, 5*time.Millisecond)
	cancel() // crash after the retry marker write but before retirement can persist

	restartWatcher := watch.NewRaceFreeFake()
	restartClient := fake.NewClientset(podForLifecycle("default", "job-0", "uid-new", "node-a"))
	restartClient.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, restartWatcher, nil
	})
	restartCtx, cancelRestart := context.WithCancel(context.Background())
	defer cancelRestart()
	restarted := NewPodLifecycleTracker(restartClient, WithPodRunsOnReconciliationObserver(func(reconciliationCtx context.Context, observed []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error) {
		active := []persistence.RunsOnAssignment{{PodID: podID, NodeID: nodeID, PodUID: observed[0].PodUID}}
		_, err := store.ReconcileRunsOn(reconciliationCtx, cluster, active, time.Now().UTC())
		return nil, err
	}))
	require.NoError(t, restarted.Start(restartCtx))
	latest, err := store.GetLatest(restartCtx)
	require.NoError(t, err)
	require.Len(t, latest.Graph.Edges, 1)
	require.Equal(t, newAssignment.ID, latest.Graph.Edges[0].ID)
}

func TestPodLifecycleTrackerRetiresSucceededAndFailedPodsAfterGrace(t *testing.T) {
	for _, phase := range []corev1.PodPhase{corev1.PodSucceeded, corev1.PodFailed} {
		t.Run(string(phase), func(t *testing.T) {
			watcher := watch.NewRaceFreeFake()
			client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-a", "node-a"))
			client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
				return true, watcher, nil
			})

			retirements := make(chan PodRunsOnRetirement, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tracker := NewPodLifecycleTracker(client,
				withPodLifecycleGraceDuration(20*time.Millisecond),
				WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
					retirements <- retirement
					return nil
				}),
			)
			require.NoError(t, tracker.Start(ctx))

			terminal := podForLifecycle("default", "job-0", "uid-a", "node-a")
			terminal.Status.Phase = phase
			watcher.Modify(terminal)

			require.Eventually(t, func() bool {
				return tracker.AllowsRunsOn("default", "job-0", "uid-a", "node-a")
			}, time.Second, 5*time.Millisecond, "terminal Pods remain active during the grace window")

			select {
			case retirement := <-retirements:
				require.Equal(t, "default", retirement.Namespace)
				require.Equal(t, "job-0", retirement.PodName)
				require.Equal(t, "uid-a", retirement.PodUID)
				require.Equal(t, "node-a", retirement.NodeName)
			case <-time.After(time.Second):
				t.Fatal("expected terminal Pod retirement after grace")
			}
			require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-a", "node-a"))
		})
	}
}

func TestPodLifecycleTrackerRetiresOldUIDWithoutRetiringReplacement(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-old", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	retirements := make(chan PodRunsOnRetirement, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client,
		withPodLifecycleGraceDuration(30*time.Millisecond),
		WithPodRunsOnRetirementObserver(func(_ context.Context, retirement PodRunsOnRetirement) error {
			retirements <- retirement
			return nil
		}),
	)
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-old", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	require.Eventually(t, func() bool {
		tracker.mu.RLock()
		defer tracker.mu.RUnlock()
		return len(tracker.retirements) == 1
	}, time.Second, 5*time.Millisecond)
	watcher.Add(podForLifecycle("default", "job-0", "uid-new", "node-a"))

	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "job-0", "uid-new", "node-a")
	}, time.Second, 5*time.Millisecond)
	select {
	case retirement := <-retirements:
		require.Equal(t, "uid-old", retirement.PodUID)
		require.Equal(t, "node-a", retirement.NodeName)
	case <-time.After(time.Second):
		t.Fatal("expected old terminal UID retirement")
	}
	require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-old", "node-a"))
	require.True(t, tracker.AllowsRunsOn("default", "job-0", "uid-new", "node-a"))
}

func TestPodLifecycleTrackerDeniesTerminalUIDAfterDeadlineBeforeTimerCallback(t *testing.T) {
	watcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "job-0", "uid-a", "node-a"))
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, watcher, nil
	})

	baseline := time.Now().UTC()
	var nowNanos atomic.Int64
	nowNanos.Store(baseline.UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client)
	tracker.nowFn = func() time.Time { return time.Unix(0, nowNanos.Load()).UTC() }
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-a", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	require.Eventually(t, func() bool {
		tracker.mu.RLock()
		defer tracker.mu.RUnlock()
		return len(tracker.retirements) == 1
	}, time.Second, 5*time.Millisecond)
	require.True(t, tracker.AllowsRunsOn("default", "job-0", "uid-a", "node-a"))

	nowNanos.Store(baseline.Add(defaultPodLifecycleGrace).UnixNano())
	require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-a", "node-a"))
}

func TestPodLifecycleTrackerFailsClosedUntilWatchRecovers(t *testing.T) {
	firstWatcher := watch.NewRaceFreeFake()
	secondWatcher := watch.NewRaceFreeFake()
	client := fake.NewClientset(podForLifecycle("default", "api-0", "uid-a", "node-a"))
	var watchCalls atomic.Int32
	client.Fake.PrependWatchReactor("pods", func(k8stesting.Action) (bool, watch.Interface, error) {
		if watchCalls.Add(1) == 1 {
			return true, firstWatcher, nil
		}
		return true, secondWatcher, nil
	})

	states := make(chan PodLifecycleState, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tracker := NewPodLifecycleTracker(client, WithPodLifecycleStateObserver(func(state PodLifecycleState) {
		states <- state
	}))
	require.False(t, tracker.AllowsRunsOn("default", "api-0", "uid-a", "node-a"))
	require.NoError(t, tracker.Start(ctx))
	require.True(t, tracker.AllowsRunsOn("default", "api-0", "uid-a", "node-a"))

	firstWatcher.Stop()
	require.Eventually(t, func() bool {
		for {
			select {
			case state := <-states:
				if !state.Ready && state.Reason == "watch_closed" {
					return true
				}
			default:
				return false
			}
		}
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "api-0", "uid-a", "node-a")
	}, 2*time.Second, 10*time.Millisecond)
}

func podForLifecycle(namespace, name, uid, node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, UID: types.UID(uid)},
		Spec:       corev1.PodSpec{NodeName: node},
	}
}
