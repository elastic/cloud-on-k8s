// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package defaults

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
)

// AppendDefaultPVCs appends defaults PVCs to a set of existing ones.
//
// The default PVCs are not appended if:
// - any PVC has been defined by the user
// - a Volume with the same .Name is found in podSpec.Volumes (e.g. emptyDir, hostPath, or a user-provided PVC)
func AppendDefaultPVCs(
	existing []corev1.PersistentVolumeClaim,
	podSpec corev1.PodSpec,
	defaults ...corev1.PersistentVolumeClaim,
) []corev1.PersistentVolumeClaim {
	// any user defined PVC shortcuts the defaulting attempt
	if len(existing) > 0 {
		return existing
	}

	names := set.Make()
	for _, volume := range podSpec.Volumes {
		names.Add(volume.Name)
	}

	for _, defaultPVC := range defaults {
		if names.Has(defaultPVC.Name) {
			continue
		}
		existing = append(existing, defaultPVC)
	}
	return existing
}
