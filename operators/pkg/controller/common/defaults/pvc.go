// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import v1 "k8s.io/api/core/v1"

// AppendDefaultPVCs appends PVCs from defaults if a PVC with the same metadata.name is not found in existing.
func AppendDefaultPVCs(
	existing []v1.PersistentVolumeClaim,
	defaults ...v1.PersistentVolumeClaim,
) []v1.PersistentVolumeClaim {
defaults:
	for _, defaultPVC := range defaults {
		for _, existingPVC := range existing {
			if existingPVC.Name == defaultPVC.Name {
				// a PVC with that name already exists, skip.
				continue defaults
			}
		}

		existing = append(existing, defaultPVC)
	}

	return existing
}
