// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package comparison

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEqual(t *testing.T) {
	tt := []struct {
		name     string
		a        runtime.Object
		b        runtime.Object
		expected bool
	}{
		{
			name: "same except for typemeta and rv",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expected: true,
		},
		{
			name: "same including typemeta and rv",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			expected: true,
		},
		{
			name: "different specs, different typemeta",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName: "node0",
						},
					},
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expected: false,
		},

		{
			name: "different specs, same typemeta",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName: "node0",
						},
					},
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expected: false,
		},
	}
	for _, tc := range tt {
		assert.Equal(t, tc.expected, Equal(tc.a, tc.b))
	}
}

func TestDiff(t *testing.T) {
	tt := []struct {
		name       string
		a          runtime.Object
		b          runtime.Object
		expectDiff bool
	}{
		{
			name: "same except for typemeta and rv",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expectDiff: false,
		},
		{
			name: "same including typemeta and rv",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
			},
			expectDiff: false,
		},
		{
			name: "different labels, same otherwise to ensure we are only ignoring resourceversion",
			a: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "sset0",
					Labels: map[string]string{"label0": "val0"},
				},
			},
			b: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "sset0",
					Labels: map[string]string{"label1": "val1"},
				},
			},
			expectDiff: true,
		},
		{
			name: "different specs, different typemeta",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName: "node0",
						},
					},
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expectDiff: true,
		},

		{
			name: "different specs, same typemeta",
			a: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "1",
				},
				Spec: appsv1.StatefulSetSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName: "node0",
						},
					},
				},
			},
			b: &appsv1.StatefulSet{
				TypeMeta: metav1.TypeMeta{
					Kind: "StatefulSet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "sset0",
					ResourceVersion: "2",
				},
			},
			expectDiff: true,
		},
	}
	for _, tc := range tt {
		diff := Diff(tc.a, tc.b)
		if tc.expectDiff {
			assert.NotEmpty(t, diff)
		} else {
			assert.Empty(t, diff)
		}
	}
}
