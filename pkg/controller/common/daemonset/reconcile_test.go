// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package daemonset

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/nodelabels/initcontainer"
)

func TestWithTemplateHash(t *testing.T) {
	d := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "daemon",
			Namespace: "ns",
		},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{},
		},
	}

	withHash := WithTemplateHash(d)
	// the label should be set
	require.NotEmpty(t, withHash.Labels[hash.TemplateHashLabelName])
	// original object should be kept unmodified
	require.Empty(t, d.Labels)

	// label should be consistent
	withSameHash := WithTemplateHash(d)
	require.Equal(t, withHash.Labels[hash.TemplateHashLabelName], withSameHash.Labels[hash.TemplateHashLabelName])

	// label should be the same if no spec changed
	withSameHash = WithTemplateHash(withSameHash)
	require.Equal(t, withHash.Labels[hash.TemplateHashLabelName], withSameHash.Labels[hash.TemplateHashLabelName])

	// label should be different if the spec changed
	d.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
		Type: appsv1.RollingUpdateDaemonSetStrategyType,
	}
	withDifferentHash := WithTemplateHash(d)
	require.NotEmpty(t, withDifferentHash.Labels[hash.TemplateHashLabelName])
	require.NotEqual(t, withHash.Labels[hash.TemplateHashLabelName], withDifferentHash.Labels[hash.TemplateHashLabelName])
}

func TestWithTemplateHash_InitContainerImageNormalization(t *testing.T) {
	managedAnnotations := map[string]string{
		initcontainer.HashAnnotation: "managed:" + initcontainer.HashVersion,
	}
	initContainerTemplate := func(image string) corev1.PodTemplateSpec {
		return corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Annotations: managedAnnotations},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: initcontainer.ContainerName, Image: image, Command: []string{"/op", "wait-for-annotations"}},
				},
				Containers: []corev1.Container{{Name: "main", Image: "app:1.0"}},
			},
		}
	}

	for _, tc := range []struct {
		name        string
		first       appsv1.DaemonSet
		second      appsv1.DaemonSet
		expectEqual bool
	}{
		{
			name: "init container image-only change does not change the hash",
			first: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"},
				Spec:       appsv1.DaemonSetSpec{Template: initContainerTemplate("eck-operator:1.0.0")},
			},
			second: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "ns"},
				Spec:       appsv1.DaemonSetSpec{Template: initContainerTemplate("eck-operator:1.0.1")},
			},
			expectEqual: true,
		},
		{
			name: "main container image change changes the hash",
			first: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ds2", Namespace: "ns"},
				Spec: appsv1.DaemonSetSpec{Template: func() corev1.PodTemplateSpec {
					tmpl := initContainerTemplate("eck-operator:1.0.0")
					tmpl.Spec.Containers[0].Image = "app:1.0"
					return tmpl
				}()},
			},
			second: appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "ds2", Namespace: "ns"},
				Spec: appsv1.DaemonSetSpec{Template: func() corev1.PodTemplateSpec {
					tmpl := initContainerTemplate("eck-operator:1.0.1")
					tmpl.Spec.Containers[0].Image = "app:2.0"
					return tmpl
				}()},
			},
			expectEqual: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h1 := WithTemplateHash(tc.first).Labels[hash.TemplateHashLabelName]
			h2 := WithTemplateHash(tc.second).Labels[hash.TemplateHashLabelName]
			if tc.expectEqual {
				assert.Equal(t, h1, h2)
			} else {
				assert.NotEqual(t, h1, h2)
			}
		})
	}
}
