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

const (
	podLifecycleRetryDelay   = 250 * time.Millisecond
	defaultPodLifecycleGrace = 60 * time.Second
)

// PodLifecycleState is emitted only when lifecycle validation becomes available
// or unavailable. It intentionally contains no Pod identity.
type PodLifecycleState struct {
	Ready          bool
	ActivePodCount int
	Reason         string
}

// PodRunsOnRetirement identifies the exact Pod UID/node assignment whose
// runs_on edge has completed its terminal grace period.
type PodRunsOnRetirement struct {
	Namespace string
	PodName   string
	PodUID    string
	NodeName  string
	RetiredAt time.Time
}

// PodLifecycleOption configures a PodLifecycleTracker.
type PodLifecycleOption func(*PodLifecycleTracker)

// WithPodLifecycleStateObserver receives lifecycle synchronization changes.
func WithPodLifecycleStateObserver(observer func(PodLifecycleState)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.observer = observer
	}
}

// WithPodRunsOnRetirementObserver receives terminal Pod UID/node retirements.
// The observer is invoked only after the fixed grace period has elapsed.
func WithPodRunsOnRetirementObserver(observer func(PodRunsOnRetirement)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.retirementObserver = observer
	}
}

// withPodLifecycleGraceDuration overrides the terminal Pod grace period for
// focused package tests. Deployed backends retain the 60-second default.
func withPodLifecycleGraceDuration(grace time.Duration) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		if grace > 0 {
			t.terminalGrace = grace
		}
	}
}

type activePod struct {
	uid      string
	nodeName string
	terminal bool
}

type pendingRetirement struct {
	pod      activePod
	deadline time.Time
}

// PodLifecycleTracker keeps the currently active Kubernetes pod UID and node
// for each namespace/name pair. It is safe for concurrent metric ingestion.
type PodLifecycleTracker struct {
	client             kubernetes.Interface
	observer           func(PodLifecycleState)
	retirementObserver func(PodRunsOnRetirement)
	terminalGrace      time.Duration
	nowFn              func() time.Time

	mu          sync.RWMutex
	ready       bool
	pods        map[string]activePod
	retirements map[string]pendingRetirement
}

// NewPodLifecycleTracker creates a tracker that must be started before use.
func NewPodLifecycleTracker(client kubernetes.Interface, opts ...PodLifecycleOption) *PodLifecycleTracker {
	t := &PodLifecycleTracker{
		client:        client,
		pods:          make(map[string]activePod),
		retirements:   make(map[string]pendingRetirement),
		terminalGrace: defaultPodLifecycleGrace,
		nowFn:         func() time.Time { return time.Now().UTC() },
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
	if !ok || pod.uid != podUID || pod.nodeName != nodeName {
		return false
	}
	if pending, ok := t.retirements[podRetirementKey(namespace, podName, podUID, nodeName)]; ok &&
		pending.pod.uid == podUID && pending.pod.nodeName == nodeName &&
		!t.nowFn().Before(pending.deadline) {
		return false
	}
	return true
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
	if !ok {
		if current, exists := t.pods[key]; exists && current.uid == string(pod.UID) {
			delete(t.pods, key)
		}
		t.mu.Unlock()
		return
	}
	t.pods[key] = active
	shouldSchedule := active.terminal
	t.mu.Unlock()
	if shouldSchedule {
		t.scheduleRetirement(key, pod.Namespace, pod.Name, active)
	}
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
	for key, pod := range pods {
		if !pod.terminal {
			continue
		}
		namespace, name := splitPodLifecycleKey(key)
		t.scheduleRetirement(key, namespace, name, pod)
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
	return activePod{
		uid:      string(pod.UID),
		nodeName: pod.Spec.NodeName,
		terminal: pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed,
	}, true
}

func podLifecycleKey(namespace, name string) string {
	return namespace + "/" + name
}

func podRetirementKey(namespace, name, uid, node string) string {
	return namespace + "/" + name + "/" + uid + "/" + node
}

func splitPodLifecycleKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}

func (t *PodLifecycleTracker) scheduleRetirement(key, namespace, name string, pod activePod) {
	retirementKey := podRetirementKey(namespace, name, pod.uid, pod.nodeName)
	t.mu.Lock()
	if _, ok := t.retirements[retirementKey]; ok {
		t.mu.Unlock()
		return
	}
	deadline := t.nowFn().Add(t.terminalGrace)
	t.retirements[retirementKey] = pendingRetirement{pod: pod, deadline: deadline}
	grace := t.terminalGrace
	t.mu.Unlock()

	time.AfterFunc(grace, func() {
		t.completeRetirement(key, retirementKey, namespace, name, pod, deadline)
	})
}

func (t *PodLifecycleTracker) completeRetirement(key, retirementKey, namespace, name string, pod activePod, deadline time.Time) {
	t.mu.Lock()
	pending, ok := t.retirements[retirementKey]
	if !ok || pending.pod.uid != pod.uid || pending.pod.nodeName != pod.nodeName || !pending.deadline.Equal(deadline) {
		t.mu.Unlock()
		return
	}
	delete(t.retirements, retirementKey)
	if current, ok := t.pods[key]; ok {
		if current.uid == pod.uid && current.nodeName == pod.nodeName {
			delete(t.pods, key)
		}
	}
	observer := t.retirementObserver
	t.mu.Unlock()

	if observer != nil {
		observer(PodRunsOnRetirement{
			Namespace: namespace,
			PodName:   name,
			PodUID:    pod.uid,
			NodeName:  pod.nodeName,
			RetiredAt: deadline,
		})
	}
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
