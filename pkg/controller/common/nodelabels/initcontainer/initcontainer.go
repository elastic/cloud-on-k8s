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
	//   "managed:<version>" – ECK supplies the image; normalize to a stable placeholder so
	//                         operator upgrades do not trigger pod rolls.
	//   absent              – feature disabled, user-supplied image, or coincidentally-named
	//                         container; no normalization; image participates in the hash unchanged.
	HashAnnotation = "eck.k8s.elastic.co/wait-for-annotations-hash"

	// HashVersion identifies the behavioral version of the wait-for-annotations init
	// container. Bump this whenever managed Pods must roll to pick up a behaviorally
	// significant waiter change.
	HashVersion = "v1"

	// ManagedHashAnnotationValue is the HashAnnotation value ECK writes when it supplies
	// the init container image. It encodes the version so that bumping HashVersion changes
	// the annotation — and therefore the workload hash — triggering a pod roll.
	ManagedHashAnnotationValue = "managed:" + HashVersion

	// ImageHashPlaceholder replaces the operator image in the pod template before hashing.
	// It is intentionally version-agnostic: the version is already captured by the annotation,
	// so the placeholder stays stable across operator upgrades and does not double-count it.
	ImageHashPlaceholder = "eck-managed-wait-for-annotations"
)

// SetHashAnnotation configures the ECK-owned HashAnnotation on template.
// When the user has already supplied a container named ContainerName with a non-empty
// image, any existing annotation is deleted so that image changes participate in the
// workload hash normally. Otherwise, it is set to ManagedHashAnnotationValue so
// NormalizeTemplateForHash can later suppress operator-upgrade-only rolls.
func SetHashAnnotation(template *corev1.PodTemplateSpec) {
	if hasUserImageOverride(template) {
		delete(template.Annotations, HashAnnotation)
		return
	}
	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}
	template.Annotations[HashAnnotation] = ManagedHashAnnotationValue
}

// NormalizeTemplateForHash returns a deep copy of in with ECK-managed wait-for-annotations
// init containers' Image replaced by ImageHashPlaceholder. Only containers whose annotation
// marks them as ECK-managed at the current HashVersion are normalized; user-supplied images
// and coincidentally-named containers (no annotation) participate in the hash unchanged.
func NormalizeTemplateForHash(in corev1.PodTemplateSpec) corev1.PodTemplateSpec {
	out := *in.DeepCopy()

	if out.Annotations[HashAnnotation] != ManagedHashAnnotationValue {
		return out
	}

	for i := range out.Spec.InitContainers {
		c := &out.Spec.InitContainers[i]
		if c.Name == ContainerName {
			c.Image = ImageHashPlaceholder
		}
	}
	return out
}

func hasUserImageOverride(template *corev1.PodTemplateSpec) bool {
	for _, c := range template.Spec.InitContainers {
		if c.Name == ContainerName && c.Image != "" {
			return true
		}
	}
	return false
}
