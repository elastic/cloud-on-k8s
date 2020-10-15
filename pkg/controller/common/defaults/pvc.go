// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import corev1 "k8s.io/api/core/v1"

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVC is not appended if:
// - a Volume with the same .Name is found in podSpec.Volumes, and that volume is not a PVC volume
// - any PVC has been defined by the user
func AppendDefaultPVCs(
	existing []corev1.PersistentVolumeClaim,
	podSpec corev1.PodSpec,
	defaults ...corev1.PersistentVolumeClaim,
) []corev1.PersistentVolumeClaim {
	// create a set of volume names that are not PVC-volumes for efficient testing
	nonPVCvolumes := map[string]struct{}{}

	for _, volume := range podSpec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// this volume is not a PVC
			nonPVCvolumes[volume.Name] = struct{}{}
		}
	}
	// any user defined PVC shortcuts the defaulting attempt
	if len(existing) > 0 {
		return existing
	}

	for _, defaultPVC := range defaults {
		if _, isNonPVCVolume := nonPVCvolumes[defaultPVC.Name]; isNonPVCVolume {
			// the corresponding volume is not a PVC
			continue
		}
		existing = append(existing, defaultPVC)
	}
	return existing
}
