package mutation

import (
	"errors"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/elasticsearch/label"
	corev1 "k8s.io/api/core/v1"
)

var (
	// ErrNotEnoughMasterEligiblePods is an error used if a master-eligible pod cannot be deleted.
	ErrNotEnoughMasterEligiblePods = errors.New("not enough master eligible pods left")
	// ErrNotEnoughDataEligiblePods is an error used if a data-eligible pod cannot be deleted.
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
	// this has the drawback of only being able to delete nodes when there is an elected master in the cluster.
	for name, pod := range podsState.RunningReady {
		if label.IsMasterNode(pod) {
			masterEligiblePods[name] = empty
		}
		if label.IsDataNode(pod) {
			dataEligiblePods[name] = empty
		}
	}

	return PodRestrictions{
		MasterNodeNames: masterEligiblePods,
		DataNodeNames:   dataEligiblePods,
	}
}

// CanDelete returns an error if the pod cannot be safely deleted
func (r *PodRestrictions) CanDelete(pod corev1.Pod) error {
	switch {
	case label.IsMasterNode(pod) && isTheOnly(pod.Name, r.MasterNodeNames):
		return ErrNotEnoughMasterEligiblePods
	case label.IsDataNode(pod) && isTheOnly(pod.Name, r.DataNodeNames):
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
