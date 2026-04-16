// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// PodTemplateResourcesOverrideWarning returns an admission warning when shorthand
// CPU/memory values overlap with resources set on the main container in pt.
// Returns "" when the main container is missing or there is no overlap.
func PodTemplateResourcesOverrideWarning(shortPath, templatePath, mainContainer string, shorthand Resources, pt corev1.PodTemplateSpec) string {
	var main *corev1.Container
	for i := range pt.Spec.Containers {
		if pt.Spec.Containers[i].Name == mainContainer {
			main = &pt.Spec.Containers[i]
			break
		}
	}
	if main == nil {
		return ""
	}
	// define a small closuer here to help with readability
	has := func(list corev1.ResourceList, name corev1.ResourceName) bool {
		_, ok := list[name]
		return ok
	}
	overlap := (shorthand.Requests.CPU != nil && has(main.Resources.Requests, corev1.ResourceCPU)) ||
		(shorthand.Requests.Memory != nil && has(main.Resources.Requests, corev1.ResourceMemory)) ||
		(shorthand.Limits.CPU != nil && has(main.Resources.Limits, corev1.ResourceCPU)) ||
		(shorthand.Limits.Memory != nil && has(main.Resources.Limits, corev1.ResourceMemory))
	if !overlap {
		return ""
	}
	return fmt.Sprintf("%s overrides CPU/memory set in %s.spec.containers[] for container %q; remove one source of truth", shortPath, templatePath, mainContainer)
}
