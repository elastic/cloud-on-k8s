// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
)

var (
	agentDeploymentFixture = agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent",
			Namespace: "test",
		},
		Spec: agentv1alpha1.AgentSpec{
			Deployment: &agentv1alpha1.DeploymentSpec{},
		},
	}

	agentDaemonsetFixture = agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent",
			Namespace: "test",
		},
		Spec: agentv1alpha1.AgentSpec{
			DaemonSet: &agentv1alpha1.DaemonSetSpec{},
		},
	}
)

func Test_volumeIsEmptyDir(t *testing.T) {
	tests := []struct {
		name string
		vols []corev1.Volume
		want bool
	}{
		{
			name: "agent-data volume as EmptyDir is true",
			vols: []corev1.Volume{
				{
					Name: DataVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			want: true,
		},
		{
			name: "agent-data volume as Hostpath is false",
			vols: []corev1.Volume{
				{
					Name: DataVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{},
					},
				},
			},
			want: false,
		},
		{
			name: "random volume as EmptyDir is false",
			vols: []corev1.Volume{
				{
					Name: "random",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			want: false,
		},
		{
			name: "empty volumes is false",
			vols: []corev1.Volume{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := volumeIsEmptyDir(tt.vols); got != tt.want {
				t.Errorf("volumeIsEmptyDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_dataVolumeEmptyDir(t *testing.T) {
	tests := []struct {
		name string
		spec *agentv1alpha1.AgentSpec
		want bool
	}{
		{
			name: "agent with deployment and no volumes defaults to false",
			spec: &agentDeploymentFixture.Spec,
			want: false,
		},
		{
			name: "agent with daemonset and no volumes defaults to false",
			spec: &agentDeploymentFixture.Spec,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dataVolumeEmptyDir(tt.spec); got != tt.want {
				t.Errorf("dataVolumeEmptyDir() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_containerRunningAsUser0(t *testing.T) {
	tests := []struct {
		name string
		spec corev1.PodSpec
		want bool
	}{
		{
			name: "empty pod spec returns false",
			spec: corev1.PodSpec{},
			want: false,
		},
		{
			name: "agent container in pod spec with no security context returns false",
			spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "agent",
					},
				},
			},
			want: false,
		},
		{
			name: "agent container in pod spec security context and RunAsUser nil returns false",
			spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "agent",
						SecurityContext: &corev1.SecurityContext{
							RunAsUser: nil,
						},
					},
				},
			},
			want: false,
		},
		{
			name: "agent container in pod spec set to run as user 1000 returns false",
			spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "agent",
						SecurityContext: &corev1.SecurityContext{
							RunAsUser: pointer.Int64(1000),
						},
					},
				},
			},
			want: false,
		},
		{
			name: "agent container in pod spec set to run as user 0 returns true",
			spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "agent",
						SecurityContext: &corev1.SecurityContext{
							RunAsUser: pointer.Int64(0),
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containerRunningAsUser0(tt.spec); got != tt.want {
				t.Errorf("containerRunningAsUser0() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_runningAsRoot(t *testing.T) {
	tests := []struct {
		name string
		spec *agentv1alpha1.AgentSpec
		want bool
	}{
		{
			name: "daemonset with no security context returns false",
			spec: &agentDaemonsetFixture.Spec,
			want: false,
		},
		{
			name: "daemonset with security context no runAsUser returns false",
			spec: withSecurityContext(agentDaemonsetFixture.Spec, &corev1.PodSecurityContext{}),
			want: false,
		},
		{
			name: "daemonset with security context runAsUser 1000 returns false",
			spec: withSecurityContext(agentDaemonsetFixture.Spec, &corev1.PodSecurityContext{
				RunAsUser: pointer.Int64(1000),
			}),
			want: false,
		},
		{
			name: "daemonset with security context runAsUser 0 returns true",
			spec: withSecurityContext(agentDaemonsetFixture.Spec, &corev1.PodSecurityContext{
				RunAsUser: pointer.Int64(0),
			}),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runningAsRoot(tt.spec); got != tt.want {
				t.Errorf("runningAsRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func withSecurityContext(spec agentv1alpha1.AgentSpec, sec *corev1.PodSecurityContext) *agentv1alpha1.AgentSpec {
	agentSpec := spec.DeepCopy()
	if agentSpec.DaemonSet != nil {
		agentSpec.DaemonSet.PodTemplate.Spec.SecurityContext = sec
		return agentSpec
	}
	if agentSpec.Deployment != nil {
		agentSpec.Deployment.PodTemplate.Spec.SecurityContext = sec
		return agentSpec
	}
	return agentSpec
}

func withVersion(spec agentv1alpha1.AgentSpec, version string) agentv1alpha1.AgentSpec {
	agentSpec := spec.DeepCopy()
	agentSpec.Version = version
	return *agentSpec
}

func Test_maybeAgentInitContainerForHostpathVolume(t *testing.T) {
	type args struct {
		params Params
		v      semver.Version
	}
	tests := []struct {
		name               string
		args               args
		wantInitContainers []corev1.Container
	}{
		{
			name: "version 7.14 does not add init container",
			args: args{
				params: Params{
					Agent: agentv1alpha1.Agent{
						Spec: withVersion(agentDaemonsetFixture.Spec, "7.14.0"),
					},
				},
				v: semver.MustParse("7.14.0"),
			},
			wantInitContainers: nil,
		},
		{
			name: "version 8.5.0 adds init container",
			args: args{
				params: Params{
					Agent: agentv1alpha1.Agent{
						Spec: withVersion(agentDaemonsetFixture.Spec, "8.5.0"),
					},
				},
				v: semver.MustParse("8.5.0"),
			},
			wantInitContainers: []corev1.Container{
				{
					Image:           "docker.elastic.co/beats/elastic-agent:8.5.0",
					Command:         hostPathVolumeInitContainerCommand(false),
					Name:            hostPathVolumeInitContainerName,
					SecurityContext: &corev1.SecurityContext{RunAsUser: pointer.Int64(0)},
					Resources:       hostPathVolumeInitContainerResources,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      DataVolumeName,
							MountPath: DataMountPath,
						},
					},
				},
			},
		},
		{
			name: "version 8.5.0 on openshift adds init container with privileged: true",
			args: args{
				params: Params{
					Agent: agentv1alpha1.Agent{
						Spec: withVersion(agentDaemonsetFixture.Spec, "8.5.0"),
					},
					OperatorParams: operator.Parameters{
						IsOpenshift: true,
					},
				},
				v: semver.MustParse("8.5.0"),
			},
			wantInitContainers: []corev1.Container{
				{
					Image:           "docker.elastic.co/beats/elastic-agent:8.5.0",
					Command:         hostPathVolumeInitContainerCommand(true),
					Name:            hostPathVolumeInitContainerName,
					SecurityContext: &corev1.SecurityContext{RunAsUser: pointer.Int64(0), Privileged: pointer.Bool(true)},
					Resources:       hostPathVolumeInitContainerResources,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      DataVolumeName,
							MountPath: DataMountPath,
						},
					},
				},
			},
		},
		{
			name: "version 8.5.0 with Emptydir Volume adds no init container",
			args: args{
				params: Params{
					Agent: agentv1alpha1.Agent{
						Spec: agentv1alpha1.AgentSpec{
							DaemonSet: &agentv1alpha1.DaemonSetSpec{
								PodTemplate: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Volumes: []corev1.Volume{
											{
												Name:         DataVolumeName,
												VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
											},
										},
									},
								},
							},
						},
					},
				},
				v: semver.MustParse("8.5.0"),
			},
			wantInitContainers: nil,
		},
		{
			name: "version 8.5.0 running as root adds no init container",
			args: args{
				params: Params{
					Agent: agentv1alpha1.Agent{
						Spec: agentv1alpha1.AgentSpec{
							DaemonSet: &agentv1alpha1.DaemonSetSpec{
								PodTemplate: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										SecurityContext: &corev1.PodSecurityContext{
											RunAsUser: pointer.Int64(0),
										},
									},
								},
							},
						},
					},
				},
				v: semver.MustParse("8.5.0"),
			},
			wantInitContainers: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotInitContainers := maybeAgentInitContainerForHostpathVolume(tt.args.params, tt.args.v); !cmp.Equal(gotInitContainers, tt.wantInitContainers) {
				t.Errorf("maybeAgentInitContainerForHostpathVolume() diff: %s", cmp.Diff(gotInitContainers, tt.wantInitContainers))
			}
		})
	}
}
