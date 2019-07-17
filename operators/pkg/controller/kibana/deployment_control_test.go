// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDeployment() appsv1.Deployment {
	return withTemplateHash(appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
			Labels:    map[string]string{"a": "b", "c": "d"},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"podlabel": "podvalue",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "container1",
							Env: []corev1.EnvVar{
								{
									Name:  "var1",
									Value: "value1",
								},
							},
						},
					},
				},
			},
		},
	})
}

func TestShouldUpdateDeployment(t *testing.T) {
	tests := []struct {
		name     string
		expected func() appsv1.Deployment
		actual   func() appsv1.Deployment
		want     bool
	}{
		{
			name:     "exact same deployment",
			expected: testDeployment,
			actual:   testDeployment,
			want:     false,
		},
		{
			name: "new expected deployment labels",
			expected: func() appsv1.Deployment {
				d := testDeployment()
				d.Labels["newlabel"] = "newvalue"
				return withTemplateHash(d)
			},
			actual: testDeployment,
			want:   true,
		},
		{
			name: "new expected pod labels",
			expected: func() appsv1.Deployment {
				d := testDeployment()
				d.Spec.Template.Labels["newlabel"] = "newvalue"
				return withTemplateHash(d)
			},
			actual: testDeployment,
			want:   true,
		},
		{
			name:     "actual deployment has additional labels: should not replace",
			expected: testDeployment,
			actual: func() appsv1.Deployment {
				d := testDeployment()
				d.Labels["newlabel"] = "newvalue"
				return d
			},
			want: false,
		},
		{
			name: "different expected container name",
			expected: func() appsv1.Deployment {
				d := testDeployment()
				d.Spec.Template.Spec.Containers[0].Name = "newContainerName"
				return withTemplateHash(d)
			},
			actual: testDeployment,
			want:   true,
		},
		{
			name: "different expected environment vars",
			expected: func() appsv1.Deployment {
				d := testDeployment()
				d.Spec.Template.Spec.Containers[0].Env = append(d.Spec.Template.Spec.Containers[0].Env,
					corev1.EnvVar{Name: "newEnvVar", Value: "value"})
				return withTemplateHash(d)
			},
			actual: testDeployment,
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldUpdateDeployment(tt.expected(), tt.actual()); got != tt.want {
				t.Errorf("ShouldUpdateDeployment() = %v, want %v", got, tt.want)
			}
		})
	}
}
