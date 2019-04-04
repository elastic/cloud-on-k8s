// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
)

// adaptChangesForFullClusterRestart returns a modified copy of the given changes,
// to prepare a full cluster restart that may include some configuration changes.
//
// The general idea is to restart all existing pods, and benefit from that restart
// to optionally pass-in a new configuration to restarted pods.
// Any larger modification (eg. a spec change requiring a new pod creation), will be
// delayed until the full cluster restart is over (hence removed from the changes).
//
// From the given changes:
// - pods to keep are turned into pods to reuse (with the same configuration)
// - attempt to find a match between pods to delete and pods to create (same spec, different config),
//   turn them into a single pod to reuse
// - remaining pods to create are removed: their actual creation will be scheduled once the full restart is over,
//   on a subsequent changes calculation
// - remaining pods to delete are also kept as pods to reuse: we want to delay their deletion until
//   the full cluster restart is over
func adaptChangesForFullClusterRestart(originalChanges Changes, targetLicense v1alpha1.LicenseType) Changes {
	changes := originalChanges.Copy()

	changes.RequireFullClusterRestart = true

	// we want to restart pods to keep: mark them for reuse instead
	for _, p := range changes.ToKeep {
		changes.ToReuse = append(changes.ToReuse, p)
	}
	changes.ToKeep = pod.PodsWithConfig{}

	// attempt to reuse some pods to delete for pods to create.
	changes = optimizeForPodReuse(changes)
	log.V(1).Info("Reusable pods with a different configuration", "count", len(changes.ToReuse))

	// don't create any new pod yet, we'll do the full restart first.
	changes.ToCreate = []PodToCreate{}

	// don't delete any pod yet: they should be restarted first with the new target license.
	for _, toDelete := range changes.ToDelete {
		log.V(1).Info("Setting pod to delete as pod to reuse", "pod", toDelete.Pod.Name)
		// remove any XPack security and self-gen license in the current config
		targetConfig := settings.DisableXPackSecurity(toDelete.Config)
		targetConfig = settings.DisableSelfGenLicense(toDelete.Config)
		// apply the correct settings for the new license
		targetConfig = targetConfig.
			MergeWith(settings.XPackSecurityConfig(targetLicense)).
			MergeWith(settings.SelfGenLicenseConfig(targetLicense))

		changes.ToReuse = append(changes.ToReuse, pod.PodWithConfig{
			Pod:    toDelete.Pod,
			Config: targetConfig,
		})
	}

	// No more deletion.
	changes.ToDelete = []pod.PodWithConfig{}

	return changes
}
