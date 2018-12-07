package mutation

import (
	"errors"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
)

var (
	// ErrNotEnoughDataEligiblePods is an error used if a master-eligible pod cannot be removed.
	ErrNotEnoughMasterEligiblePods = errors.New("not enough master eligible pods left")
	// ErrNotEnoughDataEligiblePods is an error used if a data-eligible pod cannot be removed.
	ErrNotEnoughDataEligiblePods = errors.New("not enough data eligible pods left")
)

// PodRestrictions can be used to verify that invariants around available pods are not broken.
type PodRestrictions struct {
	MasterEligiblePods map[string]corev1.Pod
	DataEligiblePods   map[string]corev1.Pod
}

// NewPodRestrictions creates a new PodRestrictions by looking at the current state of pods.
func NewPodRestrictions(podsState PodsState) PodRestrictions {
	masterEligiblePods := make(map[string]corev1.Pod)
	dataEligiblePods := make(map[string]corev1.Pod)

	for _, pods := range []map[string]corev1.Pod{
		// restrictions should only count master / data nodes that are known good
		// this has the drawback of only being able to remove nodes when there is an elected master in the cluster.
		podsState.RunningReady,
	} {
		for name, pod := range pods {
			if support.NodeTypesMasterLabelName.HasValue(true, pod.Labels) {
				masterEligiblePods[name] = pod
			}
			if support.NodeTypesDataLabelName.HasValue(true, pod.Labels) {
				dataEligiblePods[name] = pod
			}
		}
	}

	return PodRestrictions{
		MasterEligiblePods: masterEligiblePods,
		DataEligiblePods:   dataEligiblePods,
	}
}

// CanRemove returns an error if the pod cannot be safely removed.
func (r PodRestrictions) CanRemove(pod corev1.Pod) error {
	if support.NodeTypesMasterLabelName.HasValue(true, pod.Labels) {
		// this node is a master node

		_, isInEligibilitySet := r.MasterEligiblePods[pod.Name]
		currentEligible := len(r.MasterEligiblePods)

		// cannot remove:
		// - if this is in the eligible set and there isn't at least one other pod there as well
		// - if this is not in the eligible set and there isn't at least another pod there
		if !(isInEligibilitySet && currentEligible >= 2) || (!isInEligibilitySet && currentEligible >= 1) {
			return ErrNotEnoughMasterEligiblePods
		}
	}

	if support.NodeTypesDataLabelName.HasValue(true, pod.Labels) {
		// this node is a data node

		_, isInEligibilitySet := r.DataEligiblePods[pod.Name]
		currentEligible := len(r.DataEligiblePods)

		// cannot remove:
		// - if this is in the eligible set and there isn't at least one other pod there as well
		// - if this is not in the eligible set and there isn't at least another pod there
		if !(isInEligibilitySet && currentEligible > 1) || (!isInEligibilitySet && currentEligible >= 1) {
			return ErrNotEnoughDataEligiblePods
		}
	}

	// no issues found with removal
	return nil
}

// Remove removes the pod from the restrictions.
func (r *PodRestrictions) Remove(pod corev1.Pod) {
	delete(r.MasterEligiblePods, pod.Name)
	delete(r.DataEligiblePods, pod.Name)
}
