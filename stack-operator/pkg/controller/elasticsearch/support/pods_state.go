package support

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
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
	resourcesState ResourcesState,
	observedState ObservedState,
) PodsState {
	podsState := newEmptyPodsState()

	// pendingPods are pods that have been created in the API but is not scheduled or running yet.
	pendingPods, _ := resourcesState.CurrentPodsByPhase[corev1.PodPending]
	for _, pod := range pendingPods {
		podsState.Pending[pod.Name] = pod
	}

	if observedState.ClusterState != nil {
		// since we have a cluster state, attempt to categorize pods further into Joining/Ready and capture the
		// MasterNodePod

		nodesByName := make(map[string]client.Node, len(observedState.ClusterState.Nodes))
		for _, node := range observedState.ClusterState.Nodes {
			nodesByName[node.Name] = node
		}
		masterNodeName := observedState.ClusterState.MasterNodeName()

		for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodRunning] {
			// if the pod is not known in the cluster state, we assume it's supposed to join
			if _, ok := nodesByName[currentRunningPod.Name]; !ok {
				podsState.RunningJoining[currentRunningPod.Name] = currentRunningPod
			} else {
				podsState.RunningReady[currentRunningPod.Name] = currentRunningPod
			}

			if currentRunningPod.Name == masterNodeName {
				podsState.MasterNodePod = &currentRunningPod
			}
		}
	} else {
		// no cluster state was available, so all the pods go into the RunningUnknown state
		for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodRunning] {
			podsState.RunningUnknown[currentRunningPod.Name] = currentRunningPod
		}
	}

	for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodSucceeded] {
		podsState.Terminal[currentRunningPod.Name] = currentRunningPod
	}
	for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodFailed] {
		podsState.Terminal[currentRunningPod.Name] = currentRunningPod
	}
	for _, currentRunningPod := range resourcesState.CurrentPodsByPhase[corev1.PodUnknown] {
		podsState.Unknown[currentRunningPod.Name] = currentRunningPod
	}

	// deletingPods are pods we have issued a delete request for, but haven't disappeared from the API yet
	for _, pod := range resourcesState.DeletingPods {
		podsState.Deleting[pod.Name] = pod
	}

	return podsState
}

// newEmptyPodsState initializes a PodsState with empty maps.
func newEmptyPodsState() PodsState {
	return PodsState{
		Pending:        make(map[string]corev1.Pod),
		RunningJoining: make(map[string]corev1.Pod),
		RunningReady:   make(map[string]corev1.Pod),
		RunningUnknown: make(map[string]corev1.Pod),
		Unknown:        make(map[string]corev1.Pod),
		Terminal:       make(map[string]corev1.Pod),
		Deleting:       make(map[string]corev1.Pod),
	}
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

// Partition partitions the PodsState into two: one set that contains pods in the provided ChangeSet, and one set
// containing the rest.
func (s PodsState) Partition(changeSet ChangeSet) (PodsState, PodsState) {
	podsStateOfChangeSet := newEmptyPodsState()
	podsStateOfChangeSet.MasterNodePod = s.MasterNodePod

	remaining := s

	// no need to consider changeSet.ToAdd here, as they will not exist in a PodsState
	for _, pods := range [][]corev1.Pod{changeSet.ToRemove, changeSet.ToKeep} {
		var partialState PodsState
		partialState, remaining = remaining.partitionByPods(pods)
		podsStateOfChangeSet = podsStateOfChangeSet.mergeWith(partialState)
	}
	return podsStateOfChangeSet, remaining
}

// partitionByPods partitions the PodsState into two: one set that contains pods in the provided list of pods, and one
// set containing the rest
func (s PodsState) partitionByPods(pods []corev1.Pod) (PodsState, PodsState) {
	podsStateOfPods := newEmptyPodsState()
	podsStateOfPods.MasterNodePod = s.MasterNodePod

	for _, pod := range pods {
		if _, ok := s.Pending[pod.Name]; ok {
			podsStateOfPods.Pending[pod.Name] = pod
			delete(s.Pending, pod.Name)
			continue
		}
		if _, ok := s.RunningJoining[pod.Name]; ok {
			podsStateOfPods.RunningJoining[pod.Name] = pod
			delete(s.RunningJoining, pod.Name)
			continue
		}
		if _, ok := s.RunningReady[pod.Name]; ok {
			podsStateOfPods.RunningReady[pod.Name] = pod
			delete(s.RunningReady, pod.Name)
			continue
		}
		if _, ok := s.RunningUnknown[pod.Name]; ok {
			podsStateOfPods.RunningUnknown[pod.Name] = pod
			delete(s.RunningUnknown, pod.Name)
			continue
		}
		if _, ok := s.Unknown[pod.Name]; ok {
			podsStateOfPods.Unknown[pod.Name] = pod
			delete(s.Unknown, pod.Name)
			continue
		}
		if _, ok := s.Terminal[pod.Name]; ok {
			podsStateOfPods.Terminal[pod.Name] = pod
			delete(s.Terminal, pod.Name)
			continue
		}
		if _, ok := s.Deleting[pod.Name]; ok {
			podsStateOfPods.Deleting[pod.Name] = pod
			delete(s.Deleting, pod.Name)
			continue
		}
		log.Info("Unable to find pod in pods state", "pod_name", pod.Name)
	}

	return podsStateOfPods, s
}

// mergeWith merges two PodsStates into one. If some pods exist in both, values in "other" take precedence.
func (s PodsState) mergeWith(other PodsState) PodsState {
	s.MasterNodePod = other.MasterNodePod

	for k, v := range other.Pending {
		s.Pending[k] = v
	}

	for k, v := range other.RunningJoining {
		s.RunningJoining[k] = v
	}

	for k, v := range other.RunningReady {
		s.RunningReady[k] = v
	}

	for k, v := range other.RunningUnknown {
		s.RunningUnknown[k] = v
	}

	for k, v := range other.Unknown {
		s.Unknown[k] = v
	}

	for k, v := range other.Terminal {
		s.Terminal[k] = v
	}

	for k, v := range other.Deleting {
		s.Deleting[k] = v
	}

	return s
}

// PodsStateSummary contains a shorter summary of a PodsState
type PodsStateSummary struct {
	Pending           []string `json:"pending,omitempty"`
	RunningJoining    []string `json:"runningJoining,omitempty"`
	RunningReady      []string `json:"runningReady,omitempty"`
	RunningUnknown    []string `json:"runningUnknown,omitempty"`
	Unknown           []string `json:"unknown,omitempty"`
	RemovalCandidates []string `json:"removalCandidates,omitempty"`
	Terminal          []string `json:"terminal,omitempty"`
	Deleting          []string `json:"deleting,omitempty"`

	MasterNodeName string `json:"masterNodeName,omitEmpty"`
}

// Summary creates a summary of PodsState, useful for debug-level printing and troubleshooting. Beware that for large
// clusters this may still be very verbose and you might consider looking at Status() instead.
func (s PodsState) Summary() PodsStateSummary {
	summary := PodsStateSummary{}

	if s.MasterNodePod != nil {
		summary.MasterNodeName = s.MasterNodePod.Name
	}

	for k := range s.Pending {
		summary.Pending = append(summary.Pending, k)
	}

	for k := range s.RunningJoining {
		summary.RunningJoining = append(summary.RunningJoining, k)
	}

	for k := range s.RunningReady {
		summary.RunningReady = append(summary.RunningReady, k)
	}

	for k := range s.RunningUnknown {
		summary.RunningUnknown = append(summary.RunningUnknown, k)
	}

	for k := range s.Unknown {
		summary.Unknown = append(summary.Unknown, k)
	}

	for k := range s.Terminal {
		summary.Terminal = append(summary.Terminal, k)
	}

	for k := range s.Deleting {
		summary.Deleting = append(summary.Deleting, k)
	}

	return summary
}

// PodsStateStatus is a short status of a PodsState.
type PodsStateStatus struct {
	Pending           int `json:"pending,omitempty"`
	RunningJoining    int `json:"runningJoining,omitempty"`
	RunningReady      int `json:"runningReady,omitempty"`
	RunningUnknown    int `json:"runningUnknown,omitempty"`
	Unknown           int `json:"unknown,omitempty"`
	RemovalCandidates int `json:"removalCandidates,omitempty"`
	Terminal          int `json:"terminal,omitempty"`
	Deleting          int `json:"deleting,omitempty"`

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
