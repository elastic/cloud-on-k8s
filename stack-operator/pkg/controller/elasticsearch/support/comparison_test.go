package support

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ESPod(image string, cpuLimit string) corev1.Pod {
	return corev1.Pod{Spec: ESPodSpecContext(image, cpuLimit).PodSpec}
}

func ESPodSpecContext(image string, cpuLimit string) PodSpecContext {
	return PodSpecContext{
		PodSpec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Name:            "elasticsearch",
				Ports:           DefaultContainerPorts,
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

func withEnv(env []corev1.EnvVar, ps PodSpecContext) PodSpecContext {
	ps.PodSpec.Containers[0].Env = env
	return ps
}

var defaultCPULimit = "800m"
var defaultImage = "image"

func Test_podMatchesSpec(t *testing.T) {
	type args struct {
		pod   corev1.Pod
		spec  PodSpecContext
		state ResourcesState
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
				pod:  corev1.Pod{},
				spec: PodSpecContext{PodSpec: corev1.PodSpec{}},
			},
			want:               false,
			wantErr:            errors.New("No container named elasticsearch in the given pod"),
			expectedMismatches: nil,
		},
		{
			name: "Matching pod should match",
			args: args{
				pod:  ESPod(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext(defaultImage, defaultCPULimit),
			},
			want:               true,
			wantErr:            nil,
			expectedMismatches: nil,
		},
		{
			name: "Non-matching image should not match",
			args: args{
				pod:  ESPod(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext("another-image", defaultCPULimit),
			},
			want:               false,
			wantErr:            nil,
			expectedMismatches: []string{"Docker image mismatch: expected another-image, actual image"},
		},
		{
			name: "Spec has extra env var",
			args: args{
				pod: ESPod(defaultImage, defaultCPULimit),
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
				pod: corev1.Pod{
					Spec: withEnv(
						[]corev1.EnvVar{{Name: "foo", Value: "bar"}},
						ESPodSpecContext(defaultImage, defaultCPULimit),
					).PodSpec,
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
				pod: corev1.Pod{
					Spec: withEnv(
						[]corev1.EnvVar{{Name: "foo", Value: "bar"}},
						ESPodSpecContext(defaultImage, defaultCPULimit),
					).PodSpec,
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
			name: "Pod and Spec have different ignored env vars",
			args: args{
				pod: corev1.Pod{
					Spec: withEnv(
						[]corev1.EnvVar{{Name: EnvNodeName, Value: "foo"}},
						ESPodSpecContext(defaultImage, defaultCPULimit),
					).PodSpec,
				},
				spec: withEnv(
					[]corev1.EnvVar{{Name: EnvNodeName, Value: "bar"}},
					ESPodSpecContext(defaultImage, defaultCPULimit),
				),
			},
			want:               true,
			wantErr:            nil,
		},
		{
			name: "Non-matching resources should match",
			args: args{
				pod:  ESPod(defaultImage, defaultCPULimit),
				spec: ESPodSpecContext(defaultImage, "600m"),
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Different resource limits: expected ",
		},
		{
			name: "Pod is missing a PVC",
			args: args{
				pod: ESPod(defaultImage, defaultCPULimit),
				spec: PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologySpec: v1alpha1.ElasticsearchTopologySpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: v1.ObjectMeta{
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
				pod: withPVCs(ESPod(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologySpec: v1alpha1.ElasticsearchTopologySpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: v1.ObjectMeta{
									Name: "test",
								},
							},
						},
					},
				},
				state: ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: v1.ObjectMeta{Name: "claim-foo"},
						},
					},
				},
			},
			want:                      false,
			wantErr:                   nil,
			expectedMismatchesContain: "Unmatched volumeClaimTemplate: test has no match in volumes [ foo]",
		},
		{
			name: "Pod has matching PVC",
			args: args{
				pod: withPVCs(ESPod(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologySpec: v1alpha1.ElasticsearchTopologySpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: v1.ObjectMeta{
									Name: "foo",
								},
							},
						},
					},
				},
				state: ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: v1.ObjectMeta{Name: "claim-foo"},
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
				pod: withPVCs(ESPod(defaultImage, defaultCPULimit), "foo", "claim-foo"),
				spec: PodSpecContext{
					PodSpec: ESPodSpecContext(defaultImage, defaultCPULimit).PodSpec,
					TopologySpec: v1alpha1.ElasticsearchTopologySpec{
						VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
							{
								ObjectMeta: v1.ObjectMeta{
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
				state: ResourcesState{
					PVCs: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: v1.ObjectMeta{Name: "claim-foo"},
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
			match, mismatchReasons, err := podMatchesSpec(tt.args.pod, tt.args.spec, tt.args.state)
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

// withPVCs is a small utility function to add PVCs to a Pod, the varargs argument is the volume name and claim names.
func withPVCs(pod corev1.Pod, nameAndClaimNames ...string) corev1.Pod {
	lenNameAndClaimNames := len(nameAndClaimNames)

	if lenNameAndClaimNames%2 != 0 {
		panic(fmt.Sprintf("odd number of arguments passed as key-value pairs to withPVCs"))
	}

	for i := 0; i < lenNameAndClaimNames; i += 2 {
		volumeName := nameAndClaimNames[i]
		claimName := nameAndClaimNames[i+1]

		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		})
	}
	return pod
}
