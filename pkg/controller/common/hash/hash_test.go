// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package hash

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHashObject(t *testing.T) {
	// nil objects hash the same
	require.Equal(t, HashObject(nil), HashObject(nil))

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
			Labels: map[string]string{
				"a": "b",
				"c": "d",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "container",
					Env: []corev1.EnvVar{
						{
							Name:  "var1",
							Value: "value1",
						},
					},
				},
			},
		},
	}
	samePod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
			Labels: map[string]string{
				"a": "b",
				"c": "d",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "container",
					Env: []corev1.EnvVar{
						{
							Name:  "var1",
							Value: "value1",
						},
					},
				},
			},
		},
	}

	// hashes are consistent
	hash := HashObject(pod)
	// same object
	require.Equal(t, hash, HashObject(pod))
	// different object but same content
	require.Equal(t, hash, HashObject(samePod))

	// /!\ hashing an object and its pointer lead to different values
	require.NotEqual(t, hash, HashObject(&pod))

	// hashes ignore different pointer addresses
	userID := int64(123)
	securityContext1 := corev1.PodSecurityContext{RunAsUser: &userID}
	securityContext2 := corev1.PodSecurityContext{RunAsUser: &userID}
	pod.Spec.SecurityContext = &securityContext1
	hash = HashObject(pod)
	pod.Spec.SecurityContext = &securityContext2
	require.Equal(t, hash, HashObject(pod))

	// different hash on any object modification
	pod.Labels["c"] = "newvalue"
	require.NotEqual(t, hash, HashObject(pod))
}

func TestAddTemplateHashLabel(t *testing.T) {
	spec := corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "container",
				Env: []corev1.EnvVar{
					{
						Name:  "var1",
						Value: "value1",
					},
				},
			},
		},
	}
	labels := map[string]string{
		"a": "b",
		"c": "d",
	}
	expected := map[string]string{
		"a":                   "b",
		"c":                   "d",
		TemplateHashLabelName: HashObject(spec),
	}
	require.Equal(t, expected, SetTemplateHashLabel(labels, spec))
}
