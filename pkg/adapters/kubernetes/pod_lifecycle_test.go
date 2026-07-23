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
