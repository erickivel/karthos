package kubernetes

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

const podLifecycleRetryDelay = 250 * time.Millisecond

// PodLifecycleState is emitted only when lifecycle validation becomes available
// or unavailable. It intentionally contains no Pod identity.
type PodLifecycleState struct {
	Ready          bool
	ActivePodCount int
	Reason         string
}

// PodLifecycleOption configures a PodLifecycleTracker.
type PodLifecycleOption func(*PodLifecycleTracker)

// WithPodLifecycleStateObserver receives lifecycle synchronization changes.
func WithPodLifecycleStateObserver(observer func(PodLifecycleState)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.observer = observer
	}
}

type activePod struct {
	uid      string
	nodeName string
}

// PodLifecycleTracker keeps the currently active Kubernetes pod UID and node
// for each namespace/name pair. It is safe for concurrent metric ingestion.
type PodLifecycleTracker struct {
	client   kubernetes.Interface
	observer func(PodLifecycleState)

	mu    sync.RWMutex
	ready bool
	pods  map[string]activePod
}

// NewPodLifecycleTracker creates a tracker that must be started before use.
func NewPodLifecycleTracker(client kubernetes.Interface, opts ...PodLifecycleOption) *PodLifecycleTracker {
	t := &PodLifecycleTracker{
		client: client,
		pods:   make(map[string]activePod),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Start establishes an initial List+Watch cycle. It returns only after the
// tracker has an authoritative pod snapshot and an attached watch.
func (t *PodLifecycleTracker) Start(ctx context.Context) error {
	watcher, pods, err := t.listAndWatch(ctx)
	if err != nil {
		return err
	}
	t.setAvailable(pods, "initial_sync")
	go t.run(ctx, watcher)
	return nil
}

// AllowsRunsOn implements ingest.PodLifecycle. It fails closed until a
// List+Watch cycle is active and requires both UID and node identity to match.
func (t *PodLifecycleTracker) AllowsRunsOn(namespace, podName, podUID, nodeName string) bool {
	if podUID == "" || nodeName == "" {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if !t.ready {
		return false
	}
	pod, ok := t.pods[podLifecycleKey(namespace, podName)]
	return ok && pod.uid == podUID && pod.nodeName == nodeName
}

func (t *PodLifecycleTracker) run(ctx context.Context, watcher watch.Interface) {
	for {
		reason := t.consume(ctx, watcher)
		if ctx.Err() != nil {
			return
		}
		t.setUnavailable(reason)

		for {
			if !waitForRetry(ctx) {
				return
			}
			nextWatcher, pods, err := t.listAndWatch(ctx)
			if err != nil {
				continue
			}
			t.setAvailable(pods, "resynchronized")
			watcher = nextWatcher
			break
		}
	}
}

func (t *PodLifecycleTracker) listAndWatch(ctx context.Context) (watch.Interface, map[string]activePod, error) {
	if t.client == nil {
		return nil, nil, fmt.Errorf("kubernetes client is required")
	}
	podList, err := t.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("list pods: %w", err)
	}

	pods := make(map[string]activePod, len(podList.Items))
	for i := range podList.Items {
		if pod, ok := activePodFrom(&podList.Items[i]); ok {
			pods[podLifecycleKey(podList.Items[i].Namespace, podList.Items[i].Name)] = pod
		}
	}

	watcher, err := t.client.CoreV1().Pods("").Watch(ctx, metav1.ListOptions{
		ResourceVersion:     podList.ResourceVersion,
		AllowWatchBookmarks: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("watch pods: %w", err)
	}
	return watcher, pods, nil
}

func (t *PodLifecycleTracker) consume(ctx context.Context, watcher watch.Interface) string {
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return "context_cancelled"
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return "watch_closed"
			}
			switch event.Type {
			case watch.Added, watch.Modified:
				pod, ok := event.Object.(*corev1.Pod)
				if !ok {
					return "watch_invalid_object"
				}
				t.applyPod(pod)
			case watch.Deleted:
				pod, ok := event.Object.(*corev1.Pod)
				if !ok {
					return "watch_invalid_object"
				}
				t.deletePod(pod)
			case watch.Error:
				return "watch_error"
			}
		}
	}
}

func (t *PodLifecycleTracker) applyPod(pod *corev1.Pod) {
	key := podLifecycleKey(pod.Namespace, pod.Name)
	active, ok := activePodFrom(pod)
	t.mu.Lock()
	defer t.mu.Unlock()
	if !ok {
		if current, exists := t.pods[key]; exists && current.uid == string(pod.UID) {
			delete(t.pods, key)
		}
		return
	}
	t.pods[key] = active
}

func (t *PodLifecycleTracker) deletePod(pod *corev1.Pod) {
	key := podLifecycleKey(pod.Namespace, pod.Name)
	t.mu.Lock()
	defer t.mu.Unlock()
	if current, ok := t.pods[key]; ok && current.uid == string(pod.UID) {
		delete(t.pods, key)
	}
}

func (t *PodLifecycleTracker) setAvailable(pods map[string]activePod, reason string) {
	t.mu.Lock()
	t.pods = pods
	t.ready = true
	observer := t.observer
	count := len(t.pods)
	t.mu.Unlock()
	if observer != nil {
		observer(PodLifecycleState{Ready: true, ActivePodCount: count, Reason: reason})
	}
}

func (t *PodLifecycleTracker) setUnavailable(reason string) {
	t.mu.Lock()
	wasReady := t.ready
	t.ready = false
	observer := t.observer
	count := len(t.pods)
	t.mu.Unlock()
	if wasReady && observer != nil {
		observer(PodLifecycleState{Ready: false, ActivePodCount: count, Reason: reason})
	}
}

func activePodFrom(pod *corev1.Pod) (activePod, bool) {
	if pod == nil || pod.UID == "" || pod.Spec.NodeName == "" || pod.DeletionTimestamp != nil {
		return activePod{}, false
	}
	return activePod{uid: string(pod.UID), nodeName: pod.Spec.NodeName}, true
}

func podLifecycleKey(namespace, name string) string {
	return namespace + "/" + name
}

func waitForRetry(ctx context.Context) bool {
	timer := time.NewTimer(podLifecycleRetryDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
