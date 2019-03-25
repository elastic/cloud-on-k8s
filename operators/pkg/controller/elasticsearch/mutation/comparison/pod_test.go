// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/settings"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ESPodWithConfig(image string, cpuLimit string) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod:    corev1.Pod{Spec: ESPodSpecContext(image, cpuLimit).PodSpec},
		Config: settings.FlatConfig{},
	}
}

func ESPodSpecContext(image string, cpuLimit string) pod.PodSpecContext {
	return pod.PodSpecContext{
		PodSpec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elasticsearch",
				Ports:           pod.DefaultContainerPorts,
				// TODO: Hardcoded resource limits and requests
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse(cpuLimit),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				ReadinessProbe: &corev1.Probe{
					FailureThreshold:    3,
					InitialDelaySeconds: 10,
					PeriodSeconds:       10,
					SuccessThreshold:    3,
					TimeoutSeconds:      5,
					Handler: corev1.Handler{
						Exec: &corev1.ExecAction{
							Command: []string{
								"sh",
								"-c",
								"script here",
							},
						},
					},
				},
			}},
		},
	}
}

func withEnv(env []corev1.EnvVar, ps pod.PodSpecContext) pod.PodSpecContext {
	ps.PodSpec.Containers[0].Env = env
	return ps
}

var defaultCPULimit = "800m"
var defaultImage = "image"

// withPVCs is a small utility function to add PVCs to a Pod, the varargs argument is the volume name and claim names.
func withPVCs(p pod.PodWithConfig, nameAndClaimNames ...string) pod.PodWithConfig {
	lenNameAndClaimNames := len(nameAndClaimNames)

	if lenNameAndClaimNames%2 != 0 {
		panic(fmt.Sprintf("odd number of arguments passed as key-value pairs to withPVCs"))
	}

	for i := 0; i < lenNameAndClaimNames; i += 2 {
		volumeName := nameAndClaimNames[i]
		claimName := nameAndClaimNames[i+1]

		p.Pod.Spec.Volumes = append(p.Pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}
	return p
}

func Test_PodMatchesSpec(t *testing.T) {
	fs := corev1.PersistentVolumeFilesystem
	block := corev1.PersistentVolumeBlock
	type args struct {
		pod   pod.PodWithConfig
		spec  pod.PodSpecContext
		state reconcile.ResourcesState
	}
	tests := []struct {
		name                      string
		args                      args
		want                      bool
		wantErr                   error
		expectedMismatches        []string
		expectedMismatchesContain string
	}{
		{
			name: "Call with invalid specs should return an error",
			args: args{
				pod:  pod.PodWithConfig{},
				spec: pod.PodSpecContext{PodSpec: corev1.PodSpec{}},
			},
			want:               false,
			wantErr:            errors.New("No container named elasticsearch in the given pod"),
			expectedMismatches: nil,
		},
		{
			name: "Matching pod should match",
			args: args{
				pod:  ESPodWithConfig(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext(defaultImage, defaultCPULimit),
			},
			want:               true,
			wantErr:            nil,
			expectedMismatches: nil,
		},
		{
			name: "Non-matching image should not match",
			args: args{
				pod:  ESPodWithConfig(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext("another-image", defaultCPULimit),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Docker image mismatch: expected another-image, actual image"},
		},
		{
			name: "Spec has extra env var",
			args: args{
				pod: ESPodWithConfig(defaultImage, defaultCPULimit),
				spec: withEnv(
					[]corev1.EnvVar{{Name: "foo", Value: "bar"}},
					ESPodSpecContext(defaultImage, defaultCPULimit),
				),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Environment variable foo mismatch: expected [bar], actual []"},
		},
		{
			name: "Pod has extra env var",
			args: args{
				pod: pod.PodWithConfig{
					Pod: corev1.Pod{
						Spec: withEnv(
							[]corev1.EnvVar{{Name: "foo", Value: "bar"}},
							ESPodSpecContext(defaultImage, defaultCPULimit),
						).PodSpec,
					},
				},
				spec: ESPodSpecContext(defaultImage, defaultCPULimit),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Actual has additional env variables: map[foo:{foo bar nil}]"},
		},
		{
			name: "Pod and Spec have different env var contents",
			args: args{
				pod: pod.PodWithConfig{
					Pod: corev1.Pod{
						Spec: withEnv(
							[]corev1.EnvVar{{Name: "foo", Value: "bar"}},
							ESPodSpecContext(defaultImage, defaultCPULimit),
						).PodSpec,
					},
				},
				spec: withEnv(
					[]corev1.EnvVar{{Name: "foo", Value: "baz"}},
					ESPodSpecContext(defaultImage, defaultCPULimit),
				),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Environment variable foo mismatch: expected [baz], actual [bar]"},
		},
		{
			name: "Non-matching resources should match",
			args: args{
				pod:  ESPodWithConfig(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext(defaultImage, "600m"),
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Different resource limits: expected ",
		},
		{
			name: "Pod is missing a PVC",
			args: args{
				pod: ESPodWithConfig(defaultImage, defaultCPULimit),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test",
								},
							},
						},
					},
				},
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Unmatched volumeClaimTemplate: test has no match in volumes []",
		},
		{
			name: "Pod is missing a PVC, but has another",
			args: args{
				pod: withPVCs(ESPodWithConfig(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "test",
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "claim-foo"},
						},
					},
				},
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Unmatched volumeClaimTemplate: test has no match in volumes [ foo]",
		},
		{
			name: "Pod has a PVC with an empty VolumeMode",
			args: args{
				pod: withPVCs(ESPodWithConfig(defaultImage, defaultCPULimit), "data", "elasticsearch-sample-es-7gnc85w7ll-data"),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "data",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									VolumeMode: nil,
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-sample-es-7gnc85w7ll-data"},
							Spec: corev1.PersistentVolumeClaimSpec{
								VolumeMode: &fs,
							},
						},
					},
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "Pod has a PVC with a VolumeMode set to something else than default setting",
			args: args{
				pod: withPVCs(ESPodWithConfig(defaultImage, defaultCPULimit), "data", "elasticsearch-sample-es-7gnc85w7ll-data"),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "data",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									VolumeMode: &block,
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "elasticsearch-sample-es-7gnc85w7ll-data"},
							Spec: corev1.PersistentVolumeClaimSpec{
								VolumeMode: &block,
							},
						},
					},
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "Pod has matching PVC",
			args: args{
				pod: withPVCs(ESPodWithConfig(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "foo",
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "claim-foo"},
						},
					},
				},
			},
			want:    true,
			wantErr: nil,
		},
		{
			name: "Pod has matching PVC, but spec does not match",
			args: args{
				pod: withPVCs(ESPodWithConfig(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologyElement: v1alpha1.TopologyElementSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "foo",
								},
								Spec: corev1.PersistentVolumeClaimSpec{
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceStorage: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "claim-foo"},
						},
					},
				},
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Unmatched volumeClaimTemplate: foo has no match in volumes [ foo]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, mismatchReasons, err := PodMatchesSpec(tt.args.pod, tt.args.spec, tt.args.state)
			if tt.wantErr != nil {
				assert.Error(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err, "No container named elasticsearch in the given pod")
				assert.Equal(t, tt.want, match)
				if tt.expectedMismatches != nil {
					assert.EqualValues(t, tt.expectedMismatches, mismatchReasons)
				}
				if tt.expectedMismatchesContain != "" {
					assert.Contains(t, mismatchReasons[0], tt.expectedMismatchesContain)
				}
			}
		})
	}
}
