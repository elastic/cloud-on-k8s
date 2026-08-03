// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import corev1 "k8s.io/api/core/v1"

const (
	// ContainerName is the name of the init container that blocks Pod start until the
	// operator has patched the expected node-derived annotations onto the Pod.
	ContainerName = "elastic-internal-wait-for-node-labels"

	// HashAnnotation is an ECK-owned pod template annotation set by
	// MaybeAddWaitForAnnotationsInitContainer before the ECK-built init container is merged.
	// NormalizeTemplateForHash reads it to decide how to normalize the init container image:
	//   "managed:<version>" – ECK supplies the image; normalize to the identity string so
	//                         operator upgrades do not trigger pod rolls.
	//   "user"              – the user supplied the image; leave it in the hash so changes
	//                         to it are detected and reconciled.
	//   absent              – feature disabled or coincidentally-named container; no normalization.
	HashAnnotation = "eck.k8s.elastic.co/wait-for-annotations-hash"

	// HashVersion identifies the behavioral version of the wait-for-annotations init
	// container. Bump this (e.g. to "v2") when the container's command semantics change
	// in a way that requires managed pods to roll and pick up the new binary.
	HashVersion = "v1"
)

// NormalizeTemplateForHash returns a deep copy of in with ECK-managed wait-for-annotations
// init containers' Image replaced by the stable identity recorded in HashAnnotation. Only
// containers whose annotation marks them as ECK-managed are normalized; user-supplied images
// and coincidentally-named containers (no annotation) participate in the hash unchanged.
func NormalizeTemplateForHash(in corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	out := *in.DeepCopy()

	identity, ok := out.Annotations[HashAnnotation]
	if !ok {
		return out
	}

	for i := range out.Spec.InitContainers {
		c := &out.Spec.InitContainers[i]
		if c.Name != ContainerName {
			continue
		}
		if identity == "managed:"+HashVersion {
			c.Image = identity
		}
		// For user-managed images the annotation itself records the image, so the
		// actual image also stays unchanged and both contribute to the hash.
	}
	return out
}
