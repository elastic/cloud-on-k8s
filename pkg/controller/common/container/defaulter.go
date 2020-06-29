// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package container

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// Defaulter ensures that values are set if none exists in the base container.
type Defaulter interface {
	WithImage(image string) Defaulter
	WithCommand(command []string) Defaulter
	WithArgs(args []string) Defaulter
	WithPorts(ports []corev1.ContainerPort) Defaulter
	WithEnv(vars []corev1.EnvVar) Defaulter
	WithResources(resources corev1.ResourceRequirements) Defaulter
	WithVolumeMounts(volumeMounts []corev1.VolumeMount) Defaulter
	WithReadinessProbe(readinessProbe *corev1.Probe) Defaulter
	WithPreStopHook(handler *corev1.Handler) Defaulter

	// From inherits default values from an other container.
	From(other corev1.Container) Defaulter

	// Container return a copy of the resulting container.
	Container() corev1.Container
}

var _ Defaulter = &defaulter{}

type defaulter struct {
	base *corev1.Container
}

func (d defaulter) Container() corev1.Container {
	return *d.base.DeepCopy()
}

func NewDefaulter(base *corev1.Container) Defaulter {
	return &defaulter{
		base: base,
	}
}

func (d defaulter) From(other corev1.Container) Defaulter {

	if other.Lifecycle != nil {
		d.WithPreStopHook(other.Lifecycle.PreStop)
	}

	return d.
		WithImage(other.Image).
		WithCommand(other.Command).
		WithArgs(other.Args).
		WithPorts(other.Ports).
		WithEnv(other.Env).
		WithResources(other.Resources).
		WithVolumeMounts(other.VolumeMounts).
		WithReadinessProbe(other.ReadinessProbe)
}

func (d defaulter) WithCommand(command []string) Defaulter {
	if len(d.base.Command) == 0 {
		d.base.Command = command
	}
	return d
}

func (d defaulter) WithArgs(args []string) Defaulter {
	if len(d.base.Args) == 0 {
		d.base.Args = args
	}
	return d
}

func (d defaulter) WithPorts(ports []corev1.ContainerPort) Defaulter {
	for _, p := range ports {
		if !d.portExists(p.Name) {
			d.base.Ports = append(d.base.Ports, p)
		}
	}
	return d
}

// portExists checks if a port with the given name already exists in the Container.
func (d defaulter) portExists(name string) bool {
	for _, p := range d.base.Ports {
		if p.Name == name {
			return true
		}
	}
	return false
}

// WithImage sets up the Container Docker image, unless already provided.
// The default image will be used unless customImage is not empty.
func (d defaulter) WithImage(image string) Defaulter {
	if d.base.Image == "" {
		d.base.Image = image
	}
	return d
}

func (d defaulter) WithReadinessProbe(readinessProbe *corev1.Probe) Defaulter {
	if d.base.ReadinessProbe == nil {
		d.base.ReadinessProbe = readinessProbe
	}
	return d
}

// envExists checks if an env var with the given name already exists in the provided slice.
func (d defaulter) envExists(name string) bool {
	for _, v := range d.base.Env {
		if v.Name == name {
			return true
		}
	}
	return false
}

func (d defaulter) WithEnv(vars []corev1.EnvVar) Defaulter {
	for _, v := range vars {
		if !d.envExists(v.Name) {
			d.base.Env = append(d.base.Env, v)
		}
	}
	return d
}

func (d defaulter) WithResources(resources corev1.ResourceRequirements) Defaulter {
	// Ensure resources are set
	if d.base.Resources.Requests == nil && d.base.Resources.Limits == nil {
		d.base.Resources = resources
	}
	return d
}

// volumeExists checks if a volume mount with the given name already exists in the Container.
func (d defaulter) volumeMountExists(volumeMount corev1.VolumeMount) bool {
	for _, v := range d.base.VolumeMounts {
		if v.Name == volumeMount.Name || v.MountPath == volumeMount.MountPath {
			return true
		}
	}
	return false
}

func (d defaulter) WithVolumeMounts(volumeMounts []corev1.VolumeMount) Defaulter {
	for _, v := range volumeMounts {
		if !d.volumeMountExists(v) {
			d.base.VolumeMounts = append(d.base.VolumeMounts, v)
		}
	}
	// order volume mounts by name to ensure stable pod spec comparison
	sort.SliceStable(d.base.VolumeMounts, func(i, j int) bool {
		return d.base.VolumeMounts[i].Name < d.base.VolumeMounts[j].Name
	})
	return d
}

func (d defaulter) WithPreStopHook(handler *corev1.Handler) Defaulter {
	if d.base.Lifecycle == nil {
		d.base.Lifecycle = &corev1.Lifecycle{}
	}

	if d.base.Lifecycle.PreStop == nil {
		// no user-provided hook, we can use our own
		d.base.Lifecycle.PreStop = handler
	}

	return d
}
