package mutation

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/observer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	corev1 "k8s.io/api/core/v1"
)

// PodsState contains state about different pods related to a cluster.
type PodsState struct {
	// Pending contains pods in the PodPending phase
	Pending map[string]corev1.Pod
	// RunningJoining contains pods in the PodRunning phase that are NOT part of the cluster
	RunningJoining map[string]corev1.Pod
	// RunningReady contains pods in the PodRunning phase that are part of the cluster
	RunningReady map[string]corev1.Pod
	// RunningUnknown contains pods in the PodRunning phase that may or may not be part of the cluster. This usually
	// happens because we were unable to determine the current cluster state.
	RunningUnknown map[string]corev1.Pod
	// Unknown contains pods in the PodUnknown phase (e.g Kubelet is not reporting their status)
	Unknown map[string]corev1.Pod
	// Terminal contains pods in a PodFailed or PodSucceeded state.
	Terminal map[string]corev1.Pod
	// Deleting contains pods that have been deleted, but have not yet been fully processed for deletion.
	Deleting map[string]corev1.Pod

	// MasterNodePod if non-nil is the Pod that currently is the elected master. A master might still be elected even
	// if this is nil, it just means that we were unable to get it from the current observed cluster state.
	MasterNodePod *corev1.Pod
}

// NewPodsState creates a new PodsState categorizing pods based on the provided state and intended changes.
func NewPodsState(
	resourcesState reconcilehelper.ResourcesState,
	observedState observer.State,
) PodsState {
	podsState := NewEmptyPodsState()

	// pending Pods are pods that have been created in the API but are not scheduled or running yet.
	for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodPending] {
		podsState.Pending[pod.Name] = pod
	}

	if observedState.ClusterState != nil {
		// since we have a cluster state, attempt to categorize pods further into Joining/Ready and capture the
		// MasterNodePod
		nodesByName := observedState.ClusterState.NodesByNodeName()
		masterNodeName := observedState.ClusterState.MasterNodeName()

		for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodRunning] {
			if _, ok := nodesByName[pod.Name]; ok {
				// the pod is found in the cluster state, so count it as ready
				podsState.RunningReady[pod.Name] = pod
			} else {
				// if the pod is not found in the cluster state, we assume it's supposed to join
				podsState.RunningJoining[pod.Name] = pod
			}

			if pod.Name == masterNodeName {
				// create a new reference here, otherwise we would be setting the master node pod to the iterator
				masterNodePod := pod
				podsState.MasterNodePod = &masterNodePod
			}
		}
	} else {
		// no cluster state was available, so all the pods go into the RunningUnknown state
		for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodRunning] {
			podsState.RunningUnknown[pod.Name] = pod
		}
	}

	for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodSucceeded] {
		podsState.Terminal[pod.Name] = pod
	}
	for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodFailed] {
		podsState.Terminal[pod.Name] = pod
	}
	for _, pod := range resourcesState.CurrentPodsByPhase[corev1.PodUnknown] {
		podsState.Unknown[pod.Name] = pod
	}

	// deletingPods are pods we have issued a delete request for, but haven't disappeared from the API yet
	for _, pod := range resourcesState.DeletingPods {
		podsState.Deleting[pod.Name] = pod
	}

	return podsState
}

// NewEmptyPodsState initializes a PodsState with empty maps.
func NewEmptyPodsState() PodsState {
	return initializePodsState(PodsState{})
}

// initializePodsState ensures that all maps in the PodsState are non-nil
func initializePodsState(state PodsState) PodsState {
	if state.Pending == nil {
		state.Pending = make(map[string]corev1.Pod)
	}
	if state.RunningJoining == nil {
		state.RunningJoining = make(map[string]corev1.Pod)
	}
	if state.RunningReady == nil {
		state.RunningReady = make(map[string]corev1.Pod)
	}
	if state.RunningUnknown == nil {
		state.RunningUnknown = make(map[string]corev1.Pod)
	}
	if state.Unknown == nil {
		state.Unknown = make(map[string]corev1.Pod)
	}
	if state.Terminal == nil {
		state.Terminal = make(map[string]corev1.Pod)
	}
	if state.Deleting == nil {
		state.Deleting = make(map[string]corev1.Pod)
	}
	return state
}

// CurrentPodsCount returns the count of pods that might be consuming resources in the Kubernetes cluster.
func (s PodsState) CurrentPodsCount() int {
	return len(s.Pending) +
		len(s.RunningJoining) +
		len(s.RunningReady) +
		len(s.RunningUnknown) +
		len(s.Unknown) +
		len(s.Deleting)
}

// Partition partitions the PodsState into two: one set that contains pods in the provided Changes, and one set
// containing the rest.
func (s PodsState) Partition(changes Changes) (PodsState, PodsState) {
	selected := NewEmptyPodsState()
	selected.MasterNodePod = s.MasterNodePod

	remaining := s

	// no need to consider changes.ToCreate here, as they will not exist in a PodsState
	for _, pods := range [][]corev1.Pod{changes.ToDelete, changes.ToKeep} {
		var partialState PodsState
		partialState, remaining = remaining.partitionByPods(pods)
		selected.mergeFrom(partialState)
	}
	return selected, remaining
}

// partitionByPods partitions the PodsState into two:
// - one set that contains pods in the provided list of pods
// - one set containing the rest
func (s PodsState) partitionByPods(pods []corev1.Pod) (PodsState, PodsState) {
	source := s.Copy()

	selected := NewEmptyPodsState()
	selected.MasterNodePod = source.MasterNodePod

	for _, pod := range pods {
		if movePodToFrom(pod, selected.Pending, source.Pending) {
		} else if movePodToFrom(pod, selected.RunningJoining, source.RunningJoining) {
		} else if movePodToFrom(pod, selected.RunningReady, source.RunningReady) {
		} else if movePodToFrom(pod, selected.RunningUnknown, source.RunningUnknown) {
		} else if movePodToFrom(pod, selected.Unknown, source.Unknown) {
		} else if movePodToFrom(pod, selected.Terminal, source.Terminal) {
		} else if movePodToFrom(pod, selected.Deleting, source.Deleting) {
		} else {
			log.Info("Unable to find pod in pods state", "pod_name", pod.Name)
		}
	}

	return selected, source
}

// movePodToFrom moves a pod from one map to another if it existed in from, returning true if the pod was moved
func movePodToFrom(pod corev1.Pod, to, from map[string]corev1.Pod) bool {
	if _, ok := from[pod.Name]; ok {
		to[pod.Name] = pod
		delete(from, pod.Name)
		return true
	}
	return false
}

// mergeFrom merges the provided PodsState into this one. If some pods exist in both, values in "other" take precedence.
func (s *PodsState) mergeFrom(other PodsState) {
	if other.MasterNodePod != nil {
		s.MasterNodePod = other.MasterNodePod
	}

	mapCopy(s.Pending, other.Pending)
	mapCopy(s.RunningJoining, other.RunningJoining)
	mapCopy(s.RunningReady, other.RunningReady)
	mapCopy(s.RunningUnknown, other.RunningUnknown)
	mapCopy(s.Unknown, other.Unknown)
	mapCopy(s.Terminal, other.Terminal)
	mapCopy(s.Deleting, other.Deleting)
}

// PodsStateSummary contains a shorter summary of a PodsState
type PodsStateSummary struct {
	Pending        []string `json:"pending,omitempty"`
	RunningJoining []string `json:"runningJoining,omitempty"`
	RunningReady   []string `json:"runningReady,omitempty"`
	RunningUnknown []string `json:"runningUnknown,omitempty"`
	Unknown        []string `json:"unknown,omitempty"`
	Terminal       []string `json:"terminal,omitempty"`
	Deleting       []string `json:"deleting,omitempty"`

	MasterNodeName string `json:"masterNodeName,omitEmpty"`
}

// Summary creates a summary of PodsState, useful for debug-level printing and troubleshooting. Beware that for large
// clusters this may still be very verbose and you might consider looking at Status() instead.
func (s PodsState) Summary() PodsStateSummary {
	summary := PodsStateSummary{}

	if s.MasterNodePod != nil {
		summary.MasterNodeName = s.MasterNodePod.Name
	}

	summary.Pending = pod.PodMapToNames(s.Pending)
	summary.RunningJoining = pod.PodMapToNames(s.RunningJoining)
	summary.RunningReady = pod.PodMapToNames(s.RunningReady)
	summary.RunningUnknown = pod.PodMapToNames(s.RunningUnknown)
	summary.Unknown = pod.PodMapToNames(s.Unknown)
	summary.Terminal = pod.PodMapToNames(s.Terminal)
	summary.Deleting = pod.PodMapToNames(s.Deleting)

	return summary
}

// PodsStateStatus is a short status of a PodsState.
type PodsStateStatus struct {
	Pending        int `json:"pending,omitempty"`
	RunningJoining int `json:"runningJoining,omitempty"`
	RunningReady   int `json:"runningReady,omitempty"`
	RunningUnknown int `json:"runningUnknown,omitempty"`
	Unknown        int `json:"unknown,omitempty"`
	Terminal       int `json:"terminal,omitempty"`
	Deleting       int `json:"deleting,omitempty"`

	MasterNodeName string `json:"masterNodeName,omitEmpty"`
}

// Status returns a short status of the state.
func (s PodsState) Status() PodsStateStatus {
	status := PodsStateStatus{
		Pending:        len(s.Pending),
		RunningJoining: len(s.RunningJoining),
		RunningReady:   len(s.RunningReady),
		RunningUnknown: len(s.RunningUnknown),
		Unknown:        len(s.Unknown),
		Terminal:       len(s.Terminal),
		Deleting:       len(s.Deleting),
	}

	if s.MasterNodePod != nil {
		status.MasterNodeName = s.MasterNodePod.Name
	}

	return status
}

// Copy copies the PodsState. It copies the underlying maps, but not their contents.
func (s PodsState) Copy() PodsState {
	newState := PodsState{
		MasterNodePod: s.MasterNodePod,

		Pending:        make(map[string]corev1.Pod, len(s.Pending)),
		RunningJoining: make(map[string]corev1.Pod, len(s.RunningJoining)),
		RunningReady:   make(map[string]corev1.Pod, len(s.RunningReady)),
		RunningUnknown: make(map[string]corev1.Pod, len(s.RunningUnknown)),
		Unknown:        make(map[string]corev1.Pod, len(s.Unknown)),
		Terminal:       make(map[string]corev1.Pod, len(s.Terminal)),
		Deleting:       make(map[string]corev1.Pod, len(s.Deleting)),
	}

	mapCopy(newState.Pending, s.Pending)
	mapCopy(newState.RunningJoining, s.RunningJoining)
	mapCopy(newState.RunningReady, s.RunningReady)
	mapCopy(newState.RunningUnknown, s.RunningUnknown)
	mapCopy(newState.Unknown, s.Unknown)
	mapCopy(newState.Terminal, s.Terminal)
	mapCopy(newState.Deleting, s.Deleting)

	return newState
}

// HasPodsInTransientStates returns true if there are pods in transient states.
//
// Transient states are: Pending, RunningJoining, RunningUnknown, Unknown, Deleting
// Non-transient states are: RunningReady, Terminal.
func (s PodsState) HasPodsInTransientStates() bool {
	if len(s.Pending) > 0 ||
		len(s.RunningJoining) > 0 ||
		len(s.RunningUnknown) > 0 ||
		len(s.Unknown) > 0 ||
		len(s.Deleting) > 0 {
		return true
	}
	return false
}

// mapCopy copies all key/value pairs in src into dst
func mapCopy(dst, src map[string]corev1.Pod) {
	for k, v := range src {
		dst[k] = v
	}
}
