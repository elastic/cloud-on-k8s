// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package comparison

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

var es = v1alpha1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Name: "elasticsearch",
	},
}

func ESPodWithConfig(spec pod.PodSpecContext) pod.PodWithConfig {
	return pod.PodWithConfig{
		Pod: corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name.NewPodName(es.Name, v1alpha1.NodeSpec{}),
			},
			Spec: spec.PodSpec,
		},
	}
}

func ESPodSpecContext(image string, cpuLimit string) pod.PodSpecContext {
	return pod.PodSpecContext{
		PodSpec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            v1alpha1.ElasticsearchContainerName,
				Ports:           pod.DefaultContainerPorts,
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
				Env: []corev1.EnvVar{
					{
						Name:  "var1",
						Value: "value1",
					},
					{
						Name:  "var2",
						Value: "value2",
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

func withSpecHashLabel(p pod.PodWithConfig) pod.PodWithConfig {
	p.Pod.Labels = hash.SetSpecHashLabel(p.Pod.Labels, p.Pod.Spec)
	return p
}

var defaultCPULimit = "800m"
var defaultImage = "image"

// withPVCs is a small utility function to add PVCs to a Pod spec, the varargs argument is the volume name and claim names.
func withPVCs(p pod.PodSpecContext, nameAndClaimNames ...string) pod.PodSpecContext {
	lenNameAndClaimNames := len(nameAndClaimNames)

	if lenNameAndClaimNames%2 != 0 {
		panic(fmt.Sprintf("odd number of arguments passed as key-value pairs to withPVCs"))
	}

	for i := 0; i < lenNameAndClaimNames; i += 2 {
		volumeName := nameAndClaimNames[i]
		claimName := nameAndClaimNames[i+1]

		p.PodSpec.Volumes = append(p.PodSpec.Volumes, corev1.Volume{
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
			name: "Matching pod should match",
			args: args{
				pod:  withSpecHashLabel(ESPodWithConfig(ESPodSpecContext(defaultImage, defaultCPULimit))),
				spec: ESPodSpecContext(defaultImage, defaultCPULimit),
			},
			want:               true,
			wantErr:            nil,
			expectedMismatches: nil,
		},
		{
			name: "Non-matching image should not match",
			args: args{
				pod:  withSpecHashLabel(ESPodWithConfig(ESPodSpecContext(defaultImage, defaultCPULimit))),
				spec: ESPodSpecContext("another-image", defaultCPULimit),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Spec hash and running pod spec hash are not equal"},
		},
		{
			name: "Spec has different NodeSpec.Name",
			args: args{
				pod: withSpecHashLabel(pod.PodWithConfig{
					Pod: corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: name.NewPodName(es.Name, v1alpha1.NodeSpec{
								Name: "foo",
							}),
						},
						Spec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					},
				}),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						Name: "bar",
					},
				},
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Pod base name mismatch: expected elasticsearch-es-bar, actual elasticsearch-es-foo"},
		},
		{
			name: "Pod has empty NodeSpec.Name",
			args: args{
				pod: withSpecHashLabel(pod.PodWithConfig{
					Pod: corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: name.NewPodName(es.Name, v1alpha1.NodeSpec{}),
						},
						Spec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					},
				}),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						Name: "bar",
					},
				},
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Pod base name mismatch: expected elasticsearch-es-bar, actual elasticsearch-es"},
		},
		{
			name: "Non-matching resources should not match",
			args: args{
				pod:  withSpecHashLabel(ESPodWithConfig(ESPodSpecContext(defaultImage, defaultCPULimit))),
				spec: ESPodSpecContext(defaultImage, "600m"),
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Spec hash and running pod spec hash are not equal",
		},
		{
			name: "Pod is missing a PVC",
			args: args{
				pod: withSpecHashLabel(ESPodWithConfig(ESPodSpecContext(defaultImage, defaultCPULimit))),
				spec: pod.PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
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
				pod: withSpecHashLabel(
					ESPodWithConfig(
						withPVCs(
							ESPodSpecContext(defaultImage, defaultCPULimit), "foo", "claim-foo"))),
				spec: pod.PodSpecContext{
					PodSpec: withPVCs(ESPodSpecContext(defaultImage, defaultCPULimit)).PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
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
			expectedMismatchesContain: "Spec hash and running pod spec hash are not equal",
		},
		{
			name: "Pod has a PVC with an empty VolumeMode",
			args: args{
				pod: withSpecHashLabel(
					ESPodWithConfig(
						withPVCs(
							ESPodSpecContext(defaultImage, defaultCPULimit),
							volume.ElasticsearchDataVolumeName,
							"claim-name",
						))),
				spec: pod.PodSpecContext{
					PodSpec: withPVCs(
						ESPodSpecContext(defaultImage, defaultCPULimit),
						volume.ElasticsearchDataVolumeName,
						"claim-name").PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: volume.ElasticsearchDataVolumeName,
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
							ObjectMeta: metav1.ObjectMeta{
								Name: "claim-name",
							},
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
				pod: withSpecHashLabel(
					ESPodWithConfig(
						withPVCs(
							ESPodSpecContext(defaultImage, defaultCPULimit),
							volume.ElasticsearchDataVolumeName,
							"claim-name",
						))),
				spec: pod.PodSpecContext{
					PodSpec: withPVCs(
						ESPodSpecContext(defaultImage, defaultCPULimit),
						volume.ElasticsearchDataVolumeName,
						"claim-name").PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: volume.ElasticsearchDataVolumeName,
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
							ObjectMeta: metav1.ObjectMeta{
								Name: "claim-name",
							},
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
				pod: withSpecHashLabel(
					ESPodWithConfig(
						withPVCs(
							ESPodSpecContext(defaultImage, defaultCPULimit),
							"volume-name", "claim-name"),
					)),
				spec: pod.PodSpecContext{
					PodSpec: withPVCs(ESPodSpecContext(defaultImage, defaultCPULimit),
						"volume-name", "claim-name").PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "volume-name",
								},
							},
						},
					},
				},
				state: reconcile.ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{Name: "claim-name"},
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
				pod: withSpecHashLabel(
					ESPodWithConfig(
						withPVCs(
							ESPodSpecContext(defaultImage, defaultCPULimit),
							"volume-name", "claim-name"),
					)),
				spec: pod.PodSpecContext{
					PodSpec: withPVCs(
						ESPodSpecContext(defaultImage, defaultCPULimit),
						"volume-name", "claim-name").PodSpec,
					NodeSpec: v1alpha1.NodeSpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "volume-name",
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
							ObjectMeta: metav1.ObjectMeta{Name: "claim-name"},
						},
					},
				},
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Unmatched volumeClaimTemplate: volume-name has no match in volumes [ volume-name]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, mismatchReasons, err := PodMatchesSpec(es, tt.args.pod, tt.args.spec, tt.args.state)
			if tt.wantErr != nil {
				assert.Error(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err, "No container named elasticsearch in the given pod")
				assert.Equal(t, tt.want, match, mismatchReasons)
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
