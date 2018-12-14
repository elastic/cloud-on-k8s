package mutation

import (
	"errors"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
)

var (
	// ErrNotEnoughMasterEligiblePods is an error used if a master-eligible pod cannot be removed.
	ErrNotEnoughMasterEligiblePods = errors.New("not enough master eligible pods left")
	// ErrNotEnoughDataEligiblePods is an error used if a data-eligible pod cannot be removed.
	ErrNotEnoughDataEligiblePods = errors.New("not enough data eligible pods left")
)

// PodRestrictions can be used to verify that invariants around available pods are not broken.
type PodRestrictions struct {
	MasterNodeNames map[string]struct{}
	DataNodeNames   map[string]struct{}
}

// NewPodRestrictions creates a new PodRestrictions by looking at the current state of pods.
func NewPodRestrictions(podsState PodsState) PodRestrictions {
	masterEligiblePods := make(map[string]struct{})
	dataEligiblePods := make(map[string]struct{})

	// restrictions should only count master / data nodes that are known good
	// this has the drawback of only being able to remove nodes when there is an elected master in the cluster.
	for name, pod := range podsState.RunningReady {
		if support.IsMasterNode(pod) {
			masterEligiblePods[name] = empty
		}
		if support.IsDataNode(pod) {
			dataEligiblePods[name] = empty
		}
	}

	return PodRestrictions{
		MasterNodeNames: masterEligiblePods,
		DataNodeNames:   dataEligiblePods,
	}
}

// CanRemove returns an error if the pod cannot be safely removed.
func (r *PodRestrictions) CanRemove(pod corev1.Pod) error {
	switch {
	case support.IsMasterNode(pod) && isTheOnly(pod.Name, r.MasterNodeNames):
		return ErrNotEnoughMasterEligiblePods
	case support.IsDataNode(pod) && isTheOnly(pod.Name, r.DataNodeNames):
		return ErrNotEnoughDataEligiblePods
	default:
		return nil
	}
}

// isTheOnly returns true if the name is the only entry in the map
func isTheOnly(name string, fromMap map[string]struct{}) bool {
	_, exists := fromMap[name]
	if len(fromMap) == 1 && exists {
		return true
	}
	return false
}

// Remove removes the pod from the restrictions.
func (r *PodRestrictions) Remove(pod corev1.Pod) {
	delete(r.MasterNodeNames, pod.Name)
	delete(r.DataNodeNames, pod.Name)
}
