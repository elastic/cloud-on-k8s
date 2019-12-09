package comparison

import (
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestEqual(t *testing.T) {
	tt := []struct {
		name     string
		a        metav1.Object
		b        metav1.Object
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
