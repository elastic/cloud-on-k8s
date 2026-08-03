// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
)

// managedAnnotation is the annotation value ECK sets when it owns the init container image.
const managedAnnotation = "managed:" + HashVersion

func makeTemplate(initContainerImage, otherInitImage, mainImage string, initCmd []string, annotation string) corev1.PodTemplateSpec {
	t := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{Name: ContainerName, Image: initContainerImage, Command: initCmd},
				{Name: "other-init", Image: otherInitImage},
			},
			Containers: []corev1.Container{
				{Name: "main", Image: mainImage},
			},
		},
	}
	if annotation != "" {
		t.ObjectMeta = metav1.ObjectMeta{
			Annotations: map[string]string{HashAnnotation: annotation},
		}
	}
	return t
}

func TestNormalizeTemplateForHash(t *testing.T) {
	baseCmd := []string{"/op", "wait-for-annotations", "--annotation=zone"}

	for _, tc := range []struct {
		name        string
		a           corev1.PodTemplateSpec
		b           corev1.PodTemplateSpec
		expectEqual bool
	}{
		{
			name:        "ECK-managed: changing only the init container image preserves the hash",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			b:           makeTemplate("eck-operator:1.0.1", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			expectEqual: true,
		},
		{
			name:        "ECK-managed: changing another init-container image changes the hash",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			b:           makeTemplate("eck-operator:1.0.0", "other:2.0", "app:1.0", baseCmd, managedAnnotation),
			expectEqual: false,
		},
		{
			name:        "ECK-managed: changing a main-container image changes the hash",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			b:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:2.0", baseCmd, managedAnnotation),
			expectEqual: false,
		},
		{
			name:        "ECK-managed: changing init container command changes the hash",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			b:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", append(append([]string{}, baseCmd...), "--annotation=region"), managedAnnotation),
			expectEqual: false,
		},
		{
			name:        "ECK-managed: normalization does not mutate the original template",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			b:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, managedAnnotation),
			expectEqual: true,
		},
		{
			// Simulates bumping HashVersion from v1 to v2: the identity string in the annotation
			// changes, so the old and new normalized templates hash differently.
			name:        "bumping HashVersion changes the hash",
			a:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, "managed:v1"),
			b:           makeTemplate("eck-operator:1.0.0", "other:1.0", "app:1.0", baseCmd, "managed:v2"),
			expectEqual: false,
		},
		{
			// User explicitly set the init container image. The image is not normalized so it participates
			// in the workload hash via the pod spec directly; the annotation only marks it as
			// user-supplied ("user"), not the image itself.
			name:        "user-overridden init container image: image change changes the hash",
			a:           makeTemplate("my-image:1.0", "other:1.0", "app:1.0", baseCmd, "user"),
			b:           makeTemplate("my-image:2.0", "other:1.0", "app:1.0", baseCmd, "user"),
			expectEqual: false,
		},
		{
			// Without the ECK annotation (feature disabled or coincidentally named container),
			// the image is not normalized and changes to it are detected normally.
			name:        "no annotation: init container image change changes the hash",
			a:           makeTemplate("custom:1.0", "other:1.0", "app:1.0", baseCmd, ""),
			b:           makeTemplate("custom:2.0", "other:1.0", "app:1.0", baseCmd, ""),
			expectEqual: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalImage := tc.a.Spec.InitContainers[0].Image

			ha := hash.HashObject(NormalizeTemplateForHash(tc.a))
			hb := hash.HashObject(NormalizeTemplateForHash(tc.b))
			if tc.expectEqual {
				assert.Equal(t, ha, hb)
			} else {
				assert.NotEqual(t, ha, hb)
			}

			// Verify normalization never mutates the input.
			assert.Equal(t, originalImage, tc.a.Spec.InitContainers[0].Image,
				"NormalizeTemplateForHash must not mutate the original template")
		})
	}
}
