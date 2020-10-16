// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	"github.com/elastic/cloud-on-k8s/pkg/utils/set"
	corev1 "k8s.io/api/core/v1"
)

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVCs are not appended if:
// - any PVC has been defined by the user
// - a Volume with the same .Name is found in podSpec.Volumes, and that volume is not a PVC volume
func AppendDefaultPVCs(
	existing []corev1.PersistentVolumeClaim,
	podSpec corev1.PodSpec,
	defaults ...corev1.PersistentVolumeClaim,
) []corev1.PersistentVolumeClaim {
	// any user defined PVC shortcuts the defaulting attempt
	if len(existing) > 0 {
		return existing
	}

	// create a set of volume names that are not PVC-volumes
	nonPVCvolumes := set.Make()

	for _, volume := range podSpec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// this volume is not a PVC
			nonPVCvolumes.Add(volume.Name)
		}
	}

	for _, defaultPVC := range defaults {
		if nonPVCvolumes.Has(defaultPVC.Name) {
			continue
		}
		existing = append(existing, defaultPVC)
	}
	return existing
}
