// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var varFalse = false
var varTrue = true

func TestPodTemplateBuilder_setDefaults(t *testing.T) {
	tests := []struct {
		name          string
		PodTemplate   corev1.PodTemplateSpec
		containerName string
		container     *corev1.Container
		want          corev1.PodTemplateSpec
	}{
		{
			name:          "set defaults on empty pod template",
			PodTemplate:   corev1.PodTemplateSpec{},
			containerName: "mycontainer",
			want: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &varFalse,
					Containers: []corev1.Container{
						{
							Name: "mycontainer",
						},
					},
				},
			},
		},
		{
			name: "don't override user automount SA token",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &varTrue,
				},
			},
			containerName: "mycontainer",
			want: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &varTrue,
					Containers: []corev1.Container{
						{
							Name: "mycontainer",
						},
					},
				},
			},
		},
		{
			name: "append Container on after user-provided ones",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "usercontainer1",
						},
						{
							Name: "usercontainer2",
						},
					},
				},
			},
			containerName: "mycontainer",
			want: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &varFalse,
					Containers: []corev1.Container{
						{
							Name: "usercontainer1",
						},
						{
							Name: "usercontainer2",
						},
						{
							Name: "mycontainer",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &PodTemplateBuilder{
				PodTemplate:   tt.PodTemplate,
				containerName: tt.containerName,
				Container:     tt.container,
			}
			if got := b.setDefaults().PodTemplate; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.setDefaults() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithLabels(t *testing.T) {
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		labels      map[string]string
		want        map[string]string
	}{
		{
			name: "append to but don't override user provided pod template labels",
			PodTemplate: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"a": "b",
						"c": "d",
					},
				},
			},
			labels: map[string]string{
				"a": "anothervalue",
				"e": "f",
			},
			want: map[string]string{
				"a": "b",
				"c": "d",
				"e": "f",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &PodTemplateBuilder{
				PodTemplate: tt.PodTemplate,
			}
			if got := b.WithLabels(tt.labels).PodTemplate.Labels; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithLabels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithDockerImage(t *testing.T) {
	containerName := "mycontainer"
	type args struct {
		customImage  string
		defaultImage string
	}
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
		args        args
		want        string
	}{
		{
			name:        "use default image if none provided",
			podTemplate: corev1.PodTemplateSpec{},
			args: args{
				customImage:  "",
				defaultImage: "default-image",
			},
			want: "default-image",
		},
		{
			name:        "use custom image if provided",
			podTemplate: corev1.PodTemplateSpec{},
			args: args{
				customImage:  "custom-image",
				defaultImage: "default-image",
			},
			want: "custom-image",
		},
		{
			name: "use podTemplate Container image if provided",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  containerName,
							Image: "Container-image",
						},
					},
				},
			},
			args: args{
				customImage:  "custom-image",
				defaultImage: "default-image",
			},
			want: "Container-image",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.podTemplate, containerName)
			if got := b.WithDockerImage(tt.args.customImage, tt.args.defaultImage).Container.Image; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithDockerImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithReadinessProbe(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name           string
		PodTemplate    corev1.PodTemplateSpec
		readinessProbe corev1.Probe
		want           *corev1.Probe
	}{
		{
			name:        "no readiness probe in pod template: use default one",
			PodTemplate: corev1.PodTemplateSpec{},
			readinessProbe: corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/probe",
					},
				},
			},
			want: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/probe",
					},
				},
			},
		},
		{
			name: "don't override pod template readiness probe",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/user-provided",
									},
								},
							},
						},
					},
				},
			},
			readinessProbe: corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/probe",
					},
				},
			},
			want: &corev1.Probe{
				Handler: corev1.Handler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/user-provided",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithReadinessProbe(tt.readinessProbe).Container.ReadinessProbe; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithReadinessProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithResources(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		resources   corev1.ResourceRequirements
		want        corev1.ResourceRequirements
	}{
		{
			name:        "set default resources",
			PodTemplate: corev1.PodTemplateSpec{},
			resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
		},
		{
			name: "don't override user-provided resource",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
							},
						},
					},
				},
			},
			resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithResources(tt.resources).Container.Resources; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithResources() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithPorts(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		ports       []corev1.ContainerPort
		want        []corev1.ContainerPort
	}{
		{
			name:        "set default ports",
			PodTemplate: corev1.PodTemplateSpec{},
			ports: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(8080), Protocol: corev1.ProtocolTCP},
			},
			want: []corev1.ContainerPort{
				{Name: "http", ContainerPort: int32(8080), Protocol: corev1.ProtocolTCP},
			},
		},
		{
			name: "append to but don't override user provided ports",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							Ports: []corev1.ContainerPort{
								{Name: "a", ContainerPort: int32(8080), Protocol: corev1.ProtocolTCP},
								{Name: "b", ContainerPort: int32(8081), Protocol: corev1.ProtocolTCP},
								{Name: "c", ContainerPort: int32(8082), Protocol: corev1.ProtocolTCP},
							},
						},
					},
				},
			},
			ports: []corev1.ContainerPort{
				{Name: "a", ContainerPort: int32(9999), Protocol: corev1.ProtocolTCP},
				{Name: "b", ContainerPort: int32(7777), Protocol: corev1.ProtocolTCP},
				{Name: "d", ContainerPort: int32(8083), Protocol: corev1.ProtocolTCP},
			},
			want: []corev1.ContainerPort{
				{Name: "a", ContainerPort: int32(8080), Protocol: corev1.ProtocolTCP},
				{Name: "b", ContainerPort: int32(8081), Protocol: corev1.ProtocolTCP},
				{Name: "c", ContainerPort: int32(8082), Protocol: corev1.ProtocolTCP},
				{Name: "d", ContainerPort: int32(8083), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithPorts(tt.ports).Container.Ports; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithPorts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithCommand(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		command     []string
		want        []string
	}{
		{
			name:        "set default command",
			PodTemplate: corev1.PodTemplateSpec{},
			command:     []string{"my", "command"},
			want:        []string{"my", "command"},
		},
		{
			name: "don't override user-provided command",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    containerName,
							Command: []string{"user", "provided"},
						},
					},
				}},
			command: []string{"my", "command"},
			want:    []string{"user", "provided"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithCommand(tt.command).Container.Command; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithVolumes(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		volumes     []corev1.Volume
		want        []corev1.Volume
	}{
		{
			name:        "set default volumes",
			PodTemplate: corev1.PodTemplateSpec{},
			volumes:     []corev1.Volume{{Name: "vol1"}, {Name: "vol2"}},
			want:        []corev1.Volume{{Name: "vol1"}, {Name: "vol2"}},
		},
		{
			name: "append to but don't override user-provided volumes",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "vol1",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "secret1"},
							},
						},
						{
							Name: "vol2",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "secret2"},
							},
						},
					},
				},
			},
			volumes: []corev1.Volume{
				{
					Name: "vol1",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "dont-override"},
					},
				},
				{
					Name: "vol2",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "dont-override"},
					},
				},
				{
					Name: "vol3",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "secret3"},
					},
				},
			},
			want: []corev1.Volume{
				{
					Name: "vol1",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "secret1"},
					},
				},
				{
					Name: "vol2",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "secret2"},
					},
				},
				{
					Name: "vol3",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{SecretName: "secret3"},
					},
				}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithVolumes(tt.volumes...).PodTemplate.Spec.Volumes; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithVolumes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithVolumeMounts(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name         string
		PodTemplate  corev1.PodTemplateSpec
		volumeMounts []corev1.VolumeMount
		want         []corev1.VolumeMount
	}{
		{
			name:        "set default volume mounts",
			PodTemplate: corev1.PodTemplateSpec{},
			volumeMounts: []corev1.VolumeMount{
				{
					Name: "vm1",
				},
				{
					Name: "vm2",
				},
			},
			want: []corev1.VolumeMount{
				{
					Name: "vm1",
				},
				{
					Name: "vm2",
				},
			},
		},
		{
			name: "append to but don't override user-provided volume mounts",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "vm1",
									MountPath: "path1",
								},
								{
									Name:      "vm2",
									MountPath: "path2",
								},
							},
						},
					},
				},
			},
			volumeMounts: []corev1.VolumeMount{
				{
					Name:      "vm1",
					MountPath: "/dont/override",
				},
				{
					Name:      "vm2",
					MountPath: "/dont/override",
				},
				{
					Name:      "vm3",
					MountPath: "path3",
				},
			},
			want: []corev1.VolumeMount{
				{
					Name:      "vm1",
					MountPath: "path1",
				},
				{
					Name:      "vm2",
					MountPath: "path2",
				},
				{
					Name:      "vm3",
					MountPath: "path3",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithVolumeMounts(tt.volumeMounts...).Container.VolumeMounts; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithVolumeMounts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithEnv(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		vars        []corev1.EnvVar
		want        []corev1.EnvVar
	}{
		{
			name:        "set defaults",
			PodTemplate: corev1.PodTemplateSpec{},
			vars: []corev1.EnvVar{
				{Name: "var1"}, {Name: "var2"},
			},
			want: []corev1.EnvVar{
				{Name: "var1"}, {Name: "var2"},
			},
		},
		{
			name: "append to but don't override user provided env vars",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
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
						},
					},
				},
			},
			vars: []corev1.EnvVar{
				{
					Name:  "var1",
					Value: "dont override",
				},
				{
					Name:  "var2",
					Value: "dont override",
				},
				{
					Name:  "var3",
					Value: "value3",
				},
			},
			want: []corev1.EnvVar{
				{
					Name:  "var1",
					Value: "value1",
				},
				{
					Name:  "var2",
					Value: "value2",
				},
				{
					Name:  "var3",
					Value: "value3",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithEnv(tt.vars...).Container.Env; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithTerminationGracePeriod(t *testing.T) {
	period := int64(12)
	userPeriod := int64(13)
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		period      int64
		want        *int64
	}{
		{
			name:        "set default",
			PodTemplate: corev1.PodTemplateSpec{},
			period:      period,
			want:        &period,
		},
		{
			name: "don't override user-specified value",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &userPeriod,
				},
			},
			period: period,
			want:   &userPeriod,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, "")
			if got := b.WithTerminationGracePeriod(tt.period).PodTemplate.Spec.TerminationGracePeriodSeconds; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithTerminationGracePeriod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPodTemplateBuilder_WithInitContainerDefaults(t *testing.T) {
	defaultVolumeMount := corev1.VolumeMount{
		Name:      "default-volume-mount",
		MountPath: "/default",
	}
	defaultVolumeMounts := []corev1.VolumeMount{defaultVolumeMount}

	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		want        []corev1.Container
	}{
		{
			name:        "no containers to default on",
			PodTemplate: corev1.PodTemplateSpec{},
			want:        nil,
		},
		{
			name: "default but dont override image and volume mounts",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "user-init-container1",
							Image: "user-image",
						},
						{
							Name: "user-init-container2",
							VolumeMounts: []corev1.VolumeMount{{
								Name:      "foo",
								MountPath: "/foo",
							}},
						},
						{
							Name: "user-init-container3",
							VolumeMounts: []corev1.VolumeMount{{
								Name:      "bar",
								MountPath: defaultVolumeMount.MountPath,
							}},
						},
						{
							Name: "user-init-container4",
							VolumeMounts: []corev1.VolumeMount{{
								Name:      defaultVolumeMount.Name,
								MountPath: "/baz",
							}},
						},
					},
				},
			},

			want: []corev1.Container{
				{
					Name:         "user-init-container1",
					Image:        "user-image",
					VolumeMounts: defaultVolumeMounts,
				},
				{
					Name:  "user-init-container2",
					Image: "default-image",
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "foo",
						MountPath: "/foo",
					}, defaultVolumeMount,
					},
				},
				{
					Name:  "user-init-container3",
					Image: "default-image",
					// uses the same mount path as a default mount, so default mount should not be used
					VolumeMounts: []corev1.VolumeMount{{
						Name:      "bar",
						MountPath: defaultVolumeMount.MountPath,
					}},
				},
				{
					Name:  "user-init-container4",
					Image: "default-image",
					// uses the same name as a default mount, so default mount should not be used
					VolumeMounts: []corev1.VolumeMount{{
						Name:      defaultVolumeMount.Name,
						MountPath: "/baz",
					}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, "main").
				WithDockerImage("", "default-image").
				WithVolumeMounts(defaultVolumeMounts...)

			got := b.WithInitContainerDefaults().PodTemplate.Spec.InitContainers

			require.Equal(t, tt.want, got)
		})
	}
}

func TestPodTemplateBuilder_WithInitContainers(t *testing.T) {
	tests := []struct {
		name           string
		PodTemplate    corev1.PodTemplateSpec
		initContainers []corev1.Container
		want           []corev1.Container
	}{
		{
			name:           "set defaults",
			PodTemplate:    corev1.PodTemplateSpec{},
			initContainers: []corev1.Container{{Name: "init-container1"}, {Name: "init-container2"}},
			want:           []corev1.Container{{Name: "init-container1"}, {Name: "init-container2"}},
		},
		{
			name: "append to but don't override user-provided init containers",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:  "init-container1",
							Image: "image1",
						},
						{
							Name:  "init-container2",
							Image: "image2",
						},
					},
				},
			},
			initContainers: []corev1.Container{
				{
					Name:  "init-container1",
					Image: "dont-override",
				},
				{
					Name:  "init-container2",
					Image: "dont-override",
				},
				{
					Name:  "init-container3",
					Image: "image3",
				},
			},
			want: []corev1.Container{
				{
					Name:  "init-container1",
					Image: "image1",
				},
				{
					Name:  "init-container2",
					Image: "image2",
				},
				{
					Name:  "init-container3",
					Image: "image3",
				},
			},
		},
		{
			name: "prepend provided init containers",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: "user-init-container1",
						},
						{
							Name: "user-init-container2",
						},
					},
				},
			},
			initContainers: []corev1.Container{
				{
					Name:  "init-container1",
					Image: "init-image",
				},
			},
			want: []corev1.Container{
				{
					Name:  "init-container1",
					Image: "init-image",
				},
				{
					Name: "user-init-container1",
				},
				{
					Name: "user-init-container2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, "main")

			got := b.WithInitContainers(tt.initContainers...).PodTemplate.Spec.InitContainers

			require.Equal(t, tt.want, got)
		})
	}
}

func TestPodTemplateBuilder_WithMemoryLimit(t *testing.T) {
	containerName := "mycontainer"
	tests := []struct {
		name        string
		PodTemplate corev1.PodTemplateSpec
		limit       resource.Quantity
		want        corev1.ResourceRequirements
	}{
		{
			name:        "no resource provided, set defaults",
			PodTemplate: corev1.PodTemplateSpec{},
			limit:       resource.MustParse("2Gi"),
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		},
		{
			name: "cpu limit provided, but not memory, set defaults but keep cpu",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
			limit: resource.MustParse("2Gi"),
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		},
		{
			name: "resources provided by the user, don't override",
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: containerName,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
			limit: resource.MustParse("3Gi"),
			want: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPodTemplateBuilder(tt.PodTemplate, containerName)
			if got := b.WithMemoryLimit(tt.limit).Container.Resources; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PodTemplateBuilder.WithMemoryLimit() = %v, want %v", got, tt.want)
			}
		})
	}
}
