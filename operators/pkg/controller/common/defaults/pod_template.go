// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package defaults

import (
	corev1 "k8s.io/api/core/v1"
)

// PodTemplateBuilder helps with building a pod template inheriting values
// from a user-provided pod template. It focuses on building a pod with
// one main Container.
type PodTemplateBuilder struct {
	PodTemplate   corev1.PodTemplateSpec
	containerName string
	Container     *corev1.Container
}

// NewPodTemplateBuilder returns an initialized PodTemplateBuilder with some defaults.
func NewPodTemplateBuilder(base corev1.PodTemplateSpec, containerName string) *PodTemplateBuilder {
	builder := &PodTemplateBuilder{
		PodTemplate:   *base.DeepCopy(),
		containerName: containerName,
		Container:     nil, // will be set in setDefaults
	}
	return builder.setDefaults()
}

// setDefaults sets up a default Container in the pod template,
// and disables service account token auto mount.
func (b *PodTemplateBuilder) setDefaults() *PodTemplateBuilder {
	// retrieve the existing Container from the pod template
	getContainer := func() *corev1.Container {
		for i, c := range b.PodTemplate.Spec.Containers {
			if c.Name == b.containerName {
				return &b.PodTemplate.Spec.Containers[i]
			}
		}
		return nil
	}
	b.Container = getContainer()
	if b.Container == nil {
		// create the default Container if not provided by the user
		b.PodTemplate.Spec.Containers = append(b.PodTemplate.Spec.Containers, corev1.Container{Name: b.containerName})
		b.Container = getContainer()
	}

	// disable service account token auto mount, unless explicitly enabled by the user
	varFalse := false
	if b.PodTemplate.Spec.AutomountServiceAccountToken == nil {
		b.PodTemplate.Spec.AutomountServiceAccountToken = &varFalse
	}

	return b
}

// WithLabels sets the given labels, but does not override those that already exist.
func (b *PodTemplateBuilder) WithLabels(labels map[string]string) *PodTemplateBuilder {
	b.PodTemplate.Labels = SetDefaultLabels(b.PodTemplate.Labels, labels)
	return b
}

// WithDockerImage sets up the Container Docker image, unless already provided.
// The default image will be used unless customImage is not empty.
func (b *PodTemplateBuilder) WithDockerImage(customImage string, defaultImage string) *PodTemplateBuilder {
	switch {
	case b.Container.Image != "":
		// keep user-provided Container image name
	case customImage != "":
		// use user-provided custom image
		b.Container.Image = customImage
	default:
		// use default image
		b.Container.Image = defaultImage
	}
	return b
}

// WithReadinessProbe sets up the given readiness probe, unless already provided in the template.
func (b *PodTemplateBuilder) WithReadinessProbe(readinessProbe corev1.Probe) *PodTemplateBuilder {
	if b.Container.ReadinessProbe == nil {
		// no user-provided probe, use our own
		b.Container.ReadinessProbe = &readinessProbe
	}
	return b
}

// portExists checks if a port with the given name already exists in the Container.
func (b *PodTemplateBuilder) portExists(name string) bool {
	for _, p := range b.Container.Ports {
		if p.Name == name {
			return true
		}
	}
	return false
}

// WithPorts appends the given ports to the Container ports, unless already provided in the template.
func (b *PodTemplateBuilder) WithPorts(ports []corev1.ContainerPort) *PodTemplateBuilder {
	for _, p := range ports {
		if !b.portExists(p.Name) {
			b.Container.Ports = append(b.Container.Ports, p)
		}
	}
	return b
}

// WithCommand sets the given command to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithCommand(command []string) *PodTemplateBuilder {
	if len(b.Container.Command) == 0 {
		b.Container.Command = command
	}
	return b
}

// volumeExists checks if a volume with the given name already exists in the Container.
func (b *PodTemplateBuilder) volumeExists(name string) bool {
	for _, v := range b.PodTemplate.Spec.Volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

// WithVolumes appends the given volumes to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithVolumes(volumes ...corev1.Volume) *PodTemplateBuilder {
	for _, v := range volumes {
		if !b.volumeExists(v.Name) {
			b.PodTemplate.Spec.Volumes = append(b.PodTemplate.Spec.Volumes, v)
		}
	}
	return b
}

// volumeExists checks if a volume mount with the given name already exists in the Container.
func (b *PodTemplateBuilder) volumeMountExists(name string) bool {
	for _, v := range b.Container.VolumeMounts {
		if v.Name == name {
			return true
		}
	}
	return false
}

// WithVolumeMounts appends the given volume mounts to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithVolumeMounts(volumeMounts ...corev1.VolumeMount) *PodTemplateBuilder {
	for _, v := range volumeMounts {
		if !b.volumeMountExists(v.Name) {
			b.Container.VolumeMounts = append(b.Container.VolumeMounts, v)
		}
	}
	return b
}

// envExists checks if an env var with the given name already exists in the provided slice.
func (b *PodTemplateBuilder) envExists(name string) bool {
	for _, v := range b.Container.Env {
		if v.Name == name {
			return true
		}
	}
	return false
}

// WithEnv appends the given en vars to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithEnv(vars ...corev1.EnvVar) *PodTemplateBuilder {
	for _, v := range vars {
		if !b.envExists(v.Name) {
			b.Container.Env = append(b.Container.Env, v)
		}
	}
	return b
}

// WithTerminationGracePeriod sets the given termination grace period if not already specified in the template.
func (b *PodTemplateBuilder) WithTerminationGracePeriod(period int64) *PodTemplateBuilder {
	if b.PodTemplate.Spec.TerminationGracePeriodSeconds == nil {
		b.PodTemplate.Spec.TerminationGracePeriodSeconds = &period
	}
	return b
}

// initContainerExists checks if an init container with the given name already exists in the template.
func (b *PodTemplateBuilder) initContainerExists(name string) bool {
	for _, c := range b.PodTemplate.Spec.InitContainers {
		if c.Name == name {
			return true
		}
	}
	return false
}

// WithInitContainers appends the given init containers to the pod template, unless already provided by the user.
func (b *PodTemplateBuilder) WithInitContainers(initContainers ...corev1.Container) *PodTemplateBuilder {
	for _, c := range initContainers {
		if !b.initContainerExists(c.Name) {
			b.PodTemplate.Spec.InitContainers = append(b.PodTemplate.Spec.InitContainers, c)
		}
	}
	return b
}
