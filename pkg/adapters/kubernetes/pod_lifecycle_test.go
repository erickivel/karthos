package kubernetes

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
				WithPodRunsOnRetirementObserver(func(retirement PodRunsOnRetirement) {
					retirements <- retirement
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

func TestPodLifecycleTrackerDoesNotRetireReplacementForOldTerminalTimer(t *testing.T) {
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
		WithPodRunsOnRetirementObserver(func(retirement PodRunsOnRetirement) {
			retirements <- retirement
		}),
	)
	require.NoError(t, tracker.Start(ctx))

	terminal := podForLifecycle("default", "job-0", "uid-old", "node-a")
	terminal.Status.Phase = corev1.PodSucceeded
	watcher.Modify(terminal)
	watcher.Add(podForLifecycle("default", "job-0", "uid-new", "node-a"))

	require.Eventually(t, func() bool {
		return tracker.AllowsRunsOn("default", "job-0", "uid-new", "node-a")
	}, time.Second, 5*time.Millisecond)
	select {
	case retirement := <-retirements:
		t.Fatalf("old terminal timer retired replacement: %#v", retirement)
	case <-time.After(100 * time.Millisecond):
	}
	require.False(t, tracker.AllowsRunsOn("default", "job-0", "uid-old", "node-a"))
	require.True(t, tracker.AllowsRunsOn("default", "job-0", "uid-new", "node-a"))
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
