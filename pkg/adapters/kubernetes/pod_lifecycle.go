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
	defaultRetirementBackoff = 100 * time.Millisecond
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

// PodRunsOnTerminalGrace identifies the exact terminal UID/node assignment and
// its fixed retirement deadline. It is persisted so a restart can re-arm the
// deadline even if Kubernetes has already deleted or replaced the Pod.
type PodRunsOnTerminalGrace struct {
	Namespace  string
	PodName    string
	PodUID     string
	NodeName   string
	ObservedAt time.Time
	Deadline   time.Time
	Confirmed  bool
}

// PodRunsOnTerminalState reports whether storage accepted a pending terminal
// deadline or rejected it because the UID has already retired.
type PodRunsOnTerminalState uint8

const (
	PodRunsOnTerminalPending PodRunsOnTerminalState = iota
	PodRunsOnTerminalRetired
)

// PodRunsOnAssignment identifies an observed active Pod-to-Node assignment.
type PodRunsOnAssignment struct {
	Namespace string
	PodName   string
	PodUID    string
	NodeName  string
}

// PodLifecycleOption configures a PodLifecycleTracker.
type PodLifecycleOption func(*PodLifecycleTracker)

// WithPodLifecycleStateObserver receives lifecycle synchronization changes.
func WithPodLifecycleStateObserver(observer func(PodLifecycleState)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.observer = observer
	}
}

// WithPodRunsOnRetirementObserver persists terminal and direct lifecycle
// retirements. It returns an error so the tracker can retry off the watch path.
func WithPodRunsOnRetirementObserver(observer func(context.Context, PodRunsOnRetirement) error) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.retirementObserver = observer
	}
}

// WithPodRunsOnTerminalObserver persists a terminal Pod's grace deadline. The
// first write is synchronous; only a failed first write is retried off-watch.
func WithPodRunsOnTerminalObserver(observer func(context.Context, PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.terminalObserver = observer
	}
}

// WithPodRunsOnReconciliationObserver receives the authoritative initial
// Pod List/Watch assignments before the tracker begins accepting refreshes.
func WithPodRunsOnReconciliationObserver(observer func(context.Context, []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error)) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		t.reconciliationObserver = observer
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

func withPodRetirementRetry(backoff time.Duration) PodLifecycleOption {
	return func(t *PodLifecycleTracker) {
		if backoff > 0 {
			t.retirementBackoff = backoff
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
	client                 kubernetes.Interface
	observer               func(PodLifecycleState)
	terminalObserver       func(context.Context, PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error)
	retirementObserver     func(context.Context, PodRunsOnRetirement) error
	reconciliationObserver func(context.Context, []PodRunsOnAssignment) ([]PodRunsOnTerminalGrace, error)
	terminalGrace          time.Duration
	retirementBackoff      time.Duration
	nowFn                  func() time.Time

	mu              sync.RWMutex
	ready           bool
	ctx             context.Context
	pods            map[string]activePod
	retirements     map[string]pendingRetirement
	terminalPending map[string]struct{}
}

// NewPodLifecycleTracker creates a tracker that must be started before use.
func NewPodLifecycleTracker(client kubernetes.Interface, opts ...PodLifecycleOption) *PodLifecycleTracker {
	t := &PodLifecycleTracker{
		client:            client,
		pods:              make(map[string]activePod),
		retirements:       make(map[string]pendingRetirement),
		terminalPending:   make(map[string]struct{}),
		terminalGrace:     defaultPodLifecycleGrace,
		retirementBackoff: defaultRetirementBackoff,
		nowFn:             func() time.Time { return time.Now().UTC() },
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
	t.mu.Lock()
	t.ctx = ctx
	t.mu.Unlock()
	persistedGrace, err := t.reconcileInitial(ctx, pods)
	if err != nil {
		watcher.Stop()
		return err
	}
	for _, grace := range persistedGrace {
		t.restoreRetirement(grace)
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
	retirementKey := podRetirementKey(namespace, podName, podUID, nodeName)
	if _, blocked := t.terminalPending[retirementKey]; blocked {
		return false
	}
	if pending, ok := t.retirements[retirementKey]; ok &&
		pending.pod.uid == podUID && pending.pod.nodeName == nodeName {
		return t.nowFn().Before(pending.deadline)
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
	var retirement *PodRunsOnRetirement
	var terminalGrace PodRunsOnTerminalGrace
	shouldSchedule := false
	if !ok {
		if current, exists := t.pods[key]; exists && current.uid == string(pod.UID) {
			delete(t.pods, key)
			if !t.hasTerminalGraceLocked(pod.Namespace, pod.Name, current) {
				retirement = t.retireNowLocked(pod.Namespace, pod.Name, current)
			}
		}
		t.mu.Unlock()
		if retirement != nil {
			t.emitRetirement(*retirement)
		}
		return
	}
	if current, exists := t.pods[key]; exists && (current.uid != active.uid || current.nodeName != active.nodeName) {
		if !t.hasTerminalGraceLocked(pod.Namespace, pod.Name, current) {
			retirement = t.retireNowLocked(pod.Namespace, pod.Name, current)
		}
	}
	if active.terminal {
		terminalGrace = t.newTerminalGrace(pod.Namespace, pod.Name, active)
		shouldSchedule = t.beginTerminalGraceLocked(terminalGrace)
	}
	t.pods[key] = active
	t.mu.Unlock()
	if retirement != nil {
		t.emitRetirement(*retirement)
	}
	if shouldSchedule {
		t.persistAndArmTerminalGrace(key, terminalGrace)
	}
}

func (t *PodLifecycleTracker) deletePod(pod *corev1.Pod) {
	key := podLifecycleKey(pod.Namespace, pod.Name)
	t.mu.Lock()
	var retirement *PodRunsOnRetirement
	if current, ok := t.pods[key]; ok && current.uid == string(pod.UID) {
		delete(t.pods, key)
		if !t.hasTerminalGraceLocked(pod.Namespace, pod.Name, current) {
			retirement = t.retireNowLocked(pod.Namespace, pod.Name, current)
		}
	}
	t.mu.Unlock()
	if retirement != nil {
		t.emitRetirement(*retirement)
	}
}

func (t *PodLifecycleTracker) setAvailable(pods map[string]activePod, reason string) {
	t.mu.Lock()
	retirements := make([]PodRunsOnRetirement, 0)
	for key, current := range t.pods {
		namespace, name := splitPodLifecycleKey(key)
		next, ok := pods[key]
		if (!ok || next.uid != current.uid || next.nodeName != current.nodeName) && !t.hasTerminalGraceLocked(namespace, name, current) {
			if retirement := t.retireNowLocked(namespace, name, current); retirement != nil {
				retirements = append(retirements, *retirement)
			}
		}
	}
	t.pods = pods
	t.mu.Unlock()
	for _, retirement := range retirements {
		t.emitRetirement(retirement)
	}
	for key, pod := range pods {
		if !pod.terminal {
			continue
		}
		namespace, name := splitPodLifecycleKey(key)
		t.scheduleRetirement(key, namespace, name, pod)
	}
	t.mu.Lock()
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
	grace := t.newTerminalGrace(namespace, name, pod)
	if !t.beginTerminalGrace(grace) {
		return
	}
	t.persistAndArmTerminalGrace(key, grace)
}

func (t *PodLifecycleTracker) newTerminalGrace(namespace, name string, pod activePod) PodRunsOnTerminalGrace {
	observedAt := t.nowFn()
	return PodRunsOnTerminalGrace{
		Namespace:  namespace,
		PodName:    name,
		PodUID:     pod.uid,
		NodeName:   pod.nodeName,
		ObservedAt: observedAt,
		Deadline:   observedAt.Add(t.terminalGrace),
		Confirmed:  true,
	}
}

func (t *PodLifecycleTracker) persistAndArmTerminalGrace(key string, grace PodRunsOnTerminalGrace) {
	state, err := t.persistTerminalGrace(grace)
	if err != nil {
		t.retryTerminalGrace(grace)
		return
	}
	if state == PodRunsOnTerminalRetired {
		t.clearTerminalPending(grace)
		return
	}
	t.armPendingTerminalGrace(key, grace, time.Until(grace.Deadline))
}

func (t *PodLifecycleTracker) restoreRetirement(grace PodRunsOnTerminalGrace) {
	if grace.Namespace == "" || grace.PodName == "" || grace.PodUID == "" || grace.NodeName == "" || grace.Deadline.IsZero() {
		return
	}
	t.armRestoredRetirement(podLifecycleKey(grace.Namespace, grace.PodName), grace, time.Until(grace.Deadline))
}

func (t *PodLifecycleTracker) beginTerminalGrace(grace PodRunsOnTerminalGrace) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.beginTerminalGraceLocked(grace)
}

func (t *PodLifecycleTracker) beginTerminalGraceLocked(grace PodRunsOnTerminalGrace) bool {
	retirementKey := podRetirementKey(grace.Namespace, grace.PodName, grace.PodUID, grace.NodeName)
	if _, armed := t.retirements[retirementKey]; armed {
		return false
	}
	if _, pending := t.terminalPending[retirementKey]; pending {
		return false
	}
	t.terminalPending[retirementKey] = struct{}{}
	return true
}

func (t *PodLifecycleTracker) armPendingTerminalGrace(key string, grace PodRunsOnTerminalGrace, delay time.Duration) {
	t.armRetirement(key, grace, delay, true)
}

func (t *PodLifecycleTracker) armRestoredRetirement(key string, grace PodRunsOnTerminalGrace, delay time.Duration) {
	t.armRetirement(key, grace, delay, false)
}

func (t *PodLifecycleTracker) armRetirement(key string, grace PodRunsOnTerminalGrace, delay time.Duration, requirePending bool) {
	pod := activePod{uid: grace.PodUID, nodeName: grace.NodeName, terminal: true}
	retirementKey := podRetirementKey(grace.Namespace, grace.PodName, pod.uid, pod.nodeName)
	t.mu.Lock()
	if requirePending {
		if _, pending := t.terminalPending[retirementKey]; !pending {
			t.mu.Unlock()
			return
		}
	}
	if _, ok := t.retirements[retirementKey]; ok {
		t.mu.Unlock()
		return
	}
	delete(t.terminalPending, retirementKey)
	t.retirements[retirementKey] = pendingRetirement{pod: pod, deadline: grace.Deadline}
	t.mu.Unlock()
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		t.completeRetirement(key, retirementKey, grace.Namespace, grace.PodName, pod, grace.Deadline)
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
	t.mu.Unlock()
	t.emitRetirement(PodRunsOnRetirement{
		Namespace: namespace,
		PodName:   name,
		PodUID:    pod.uid,
		NodeName:  pod.nodeName,
		RetiredAt: deadline,
	})
}

func (t *PodLifecycleTracker) retireNowLocked(namespace, name string, pod activePod) *PodRunsOnRetirement {
	retirementKey := podRetirementKey(namespace, name, pod.uid, pod.nodeName)
	delete(t.retirements, retirementKey)
	delete(t.terminalPending, retirementKey)
	retirement := PodRunsOnRetirement{
		Namespace: namespace,
		PodName:   name,
		PodUID:    pod.uid,
		NodeName:  pod.nodeName,
		RetiredAt: t.nowFn(),
	}
	return &retirement
}

func (t *PodLifecycleTracker) hasTerminalGraceLocked(namespace, name string, pod activePod) bool {
	_, ok := t.retirements[podRetirementKey(namespace, name, pod.uid, pod.nodeName)]
	return ok
}

func (t *PodLifecycleTracker) reconcileInitial(ctx context.Context, pods map[string]activePod) ([]PodRunsOnTerminalGrace, error) {
	t.mu.RLock()
	observer := t.reconciliationObserver
	t.mu.RUnlock()
	if observer == nil {
		return nil, nil
	}
	assignments := make([]PodRunsOnAssignment, 0, len(pods))
	for key, pod := range pods {
		namespace, name := splitPodLifecycleKey(key)
		assignments = append(assignments, PodRunsOnAssignment{Namespace: namespace, PodName: name, PodUID: pod.uid, NodeName: pod.nodeName})
	}
	return observer(ctx, assignments)
}

func (t *PodLifecycleTracker) persistTerminalGrace(grace PodRunsOnTerminalGrace) (PodRunsOnTerminalState, error) {
	t.mu.RLock()
	observer := t.terminalObserver
	ctx := t.ctx
	t.mu.RUnlock()
	if observer == nil {
		return PodRunsOnTerminalPending, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return observer(ctx, grace)
}

func (t *PodLifecycleTracker) clearTerminalPending(grace PodRunsOnTerminalGrace) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.terminalPending, podRetirementKey(grace.Namespace, grace.PodName, grace.PodUID, grace.NodeName))
}

func (t *PodLifecycleTracker) retryTerminalGrace(grace PodRunsOnTerminalGrace) {
	t.mu.RLock()
	observer := t.terminalObserver
	ctx := t.ctx
	backoff := t.retirementBackoff
	t.mu.RUnlock()
	if observer == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// A retry proves only that a marker was eventually written; it cannot prove
	// the initial synchronous write completed before the terminal transition.
	// Persist it as unconfirmed so restart reconciliation can fail closed when
	// this UID is no longer in the authoritative Pod List/Watch state.
	retryGrace := grace
	retryGrace.Confirmed = false
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if state, err := observer(ctx, retryGrace); err == nil {
				if state == PodRunsOnTerminalRetired {
					t.clearTerminalPending(retryGrace)
					return
				}
				t.armPendingTerminalGrace(podLifecycleKey(retryGrace.Namespace, retryGrace.PodName), retryGrace, time.Until(retryGrace.Deadline))
				return
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (t *PodLifecycleTracker) emitRetirement(retirement PodRunsOnRetirement) {
	t.mu.RLock()
	observer := t.retirementObserver
	ctx := t.ctx
	backoff := t.retirementBackoff
	t.mu.RUnlock()
	if observer == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			if err := observer(ctx, retirement); err == nil {
				return
			}
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
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
