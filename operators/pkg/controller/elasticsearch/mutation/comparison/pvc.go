package comparison

import (
	"fmt"
	"reflect"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	corev1 "k8s.io/api/core/v1"
)

// volumeAndPVC holds a volume and a PVC
type volumeAndPVC struct {
	volume corev1.Volume
	pvc    corev1.PersistentVolumeClaim
}

// comparePersistentVolumeClaims returns true if the expected persistent volume claims are found in the list of volumes
func comparePersistentVolumeClaims(
	actual []corev1.Volume,
	expected []corev1.PersistentVolumeClaim,
	state reconcile.ResourcesState,
) Comparison {
	// TODO: handle extra PVCs that are in volumes, but not in expected claim templates

	var volumeAndPVCs []volumeAndPVC
	for _, volume := range actual {
		if volume.PersistentVolumeClaim == nil {
			continue
		}
		claimName := volume.PersistentVolumeClaim.ClaimName

		pvc, err := state.FindPVCByName(claimName)
		if err != nil {
			// this is rather unexpected, and we have two options:
			// 1. return the error to our caller (possibly changing our signature a little)
			// 2. consider the pod not matching
			// we usually expect all claims from a pod to exist, so we consider this case exceptional, but we chose to
			// go with option 2 because we'd rather see the pod be replaced than potentially getting stuck in the
			// reconciliation loop without being able to reconcile further. we also chose to log it as an error level
			// to call more attention to the fact this was occurring because we would like to try to get a better
			// understanding of the scenarios in which this may happen.
			msg := "Volume is referring to unknown PVC"
			log.Error(err, msg)
			return ComparisonMismatch(fmt.Sprintf("%s: %s", msg, err))
		}

		volumeAndPVCs = append(volumeAndPVCs, volumeAndPVC{volume: volume, pvc: pvc})
	}

ExpectedTemplates:
	for _, pvcTemplate := range expected {
		for i, actualVolumeAndPVC := range volumeAndPVCs {
			if templateMatchesActualVolumeAndPvc(pvcTemplate, actualVolumeAndPVC) {
				// remove the current from the remaining volumes so it cannot be used to match another template
				volumeAndPVCs = append(volumeAndPVCs[:i], volumeAndPVCs[i+1:]...)

				// continue the outer loop because this pvc template had a match
				continue ExpectedTemplates
			}
		}

		// at this point, we were unable to match the template with any of the volumes, so the comparison should not
		// match

		volumeNames := make([]string, len(volumeAndPVCs))
		for _, avp := range volumeAndPVCs {
			volumeNames = append(volumeNames, avp.volume.Name)
		}

		return ComparisonMismatch(fmt.Sprintf(
			"Unmatched volumeClaimTemplate: %s has no match in volumes %v",
			pvcTemplate.Name,
			volumeNames,
		))
	}

	return ComparisonMatch
}

// templateMatchesActualVolumeAndPvc returns true if the pvc matches the volumeAndPVC
func templateMatchesActualVolumeAndPvc(pvcTemplate corev1.PersistentVolumeClaim, actualVolumeAndPVC volumeAndPVC) bool {

	if actualVolumeAndPVC.pvc.DeletionTimestamp != nil {
		// PVC is being deleted
		return false
	}

	if pvcTemplate.Name != actualVolumeAndPVC.volume.Name {
		// name from template does not match actual, no match
		return false
	}

	// labels
	for templateLabelKey, templateLabelValue := range pvcTemplate.Labels {
		if actualValue, ok := actualVolumeAndPVC.pvc.Labels[templateLabelKey]; !ok {
			// actual is missing a key, no match
			return false
		} else if templateLabelValue != actualValue {
			// values differ, no match
			return false
		}
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.AccessModes, actualVolumeAndPVC.pvc.Spec.AccessModes) {
		return false
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.Resources, actualVolumeAndPVC.pvc.Spec.Resources) {
		return false
	}

	// this may be set to nil to be defaulted, so here we're assuming that the storage class name
	// may have been defaulted. this may cause an unintended match, which can be worked around by
	// being explicit in the pvc template spec.
	if pvcTemplate.Spec.StorageClassName != nil &&
		!reflect.DeepEqual(pvcTemplate.Spec.StorageClassName, actualVolumeAndPVC.pvc.Spec.StorageClassName) {
		return false
	}

	if pvcTemplate.Spec.VolumeMode != nil &&
		!reflect.DeepEqual(pvcTemplate.Spec.VolumeMode, actualVolumeAndPVC.pvc.Spec.VolumeMode) {
		return false
	}

	if !reflect.DeepEqual(pvcTemplate.Spec.Selector, actualVolumeAndPVC.pvc.Spec.Selector) {
		return false
	}

	return true
}
