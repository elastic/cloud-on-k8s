// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import v1 "k8s.io/api/core/v1"

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVC is not appended if:
// - a Volume with the same .Name is found in podSpec.Volumes, and that volume is not a PVC volume
// - a PVC with the same .Metadata.Name is found in existing.
func AppendDefaultPVCs(
	existing []v1.PersistentVolumeClaim,
	podSpec v1.PodSpec,
	defaults ...v1.PersistentVolumeClaim,
) []v1.PersistentVolumeClaim {
	// create a set of volume names that are not PVC-volumes for efficient testing
	nonPVCvolumes := map[string]struct{}{}

	for _, volume := range podSpec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// this volume is not a PVC
			nonPVCvolumes[volume.Name] = struct{}{}
		}
	}

defaults:
	for _, defaultPVC := range defaults {
		for _, existingPVC := range existing {
			if existingPVC.Name == defaultPVC.Name {
				// a PVC with that name already exists, skip.
				continue defaults
			}
			if _, isNonPVCVolume := nonPVCvolumes[defaultPVC.Name]; isNonPVCVolume {
				// the corresponding volume is not a PVC
				continue defaults
			}
		}

		existing = append(existing, defaultPVC)
	}

	return existing
}
