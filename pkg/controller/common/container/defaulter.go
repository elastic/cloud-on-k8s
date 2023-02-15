// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// Defaulter ensures that values are set if none exists in the base container.
type Defaulter struct {
	base *corev1.Container
}

// Container returns a copy of the resulting container.
func (d Defaulter) Container() corev1.Container {
	return *d.base.DeepCopy()
}

func NewDefaulter(base *corev1.Container) Defaulter {
	return Defaulter{
		base: base,
	}
}

// From inherits default values from an other container.
func (d Defaulter) From(other corev1.Container) Defaulter {
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

func (d Defaulter) WithCommand(command []string) Defaulter {
	if len(d.base.Command) == 0 {
		d.base.Command = command
	}
	return d
}

func (d Defaulter) WithArgs(args []string) Defaulter {
	if len(d.base.Args) == 0 {
		d.base.Args = args
	}
	return d
}

func (d Defaulter) WithPorts(ports []corev1.ContainerPort) Defaulter {
	for _, p := range ports {
		if !d.portExists(p.Name) {
			d.base.Ports = append(d.base.Ports, p)
		}
	}
	// order ports by name to ensure stable pod spec comparison
	sort.SliceStable(d.base.Ports, func(i, j int) bool {
		return d.base.Ports[i].Name < d.base.Ports[j].Name
	})
	return d
}

// portExists checks if a port with the given name already exists in the Container.
func (d Defaulter) portExists(name string) bool {
	for _, p := range d.base.Ports {
		if p.Name == name {
			return true
		}
	}
	return false
}

// WithImage sets up the Container Docker image, unless already provided.
// The default image will be used unless customImage is not empty.
func (d Defaulter) WithImage(image string) Defaulter {
	if d.base.Image == "" {
		d.base.Image = image
	}
	return d
}

func (d Defaulter) WithReadinessProbe(readinessProbe *corev1.Probe) Defaulter {
	if d.base.ReadinessProbe == nil {
		d.base.ReadinessProbe = readinessProbe
	}
	return d
}

func (d Defaulter) WithLivenessProbe(livenessProbe *corev1.Probe) Defaulter {
	if d.base.LivenessProbe == nil {
		d.base.LivenessProbe = livenessProbe
	}
	return d
}

// envExists checks if an env var with the given name already exists in the provided slice.
func (d Defaulter) envExists(name string) bool {
	for _, v := range d.base.Env {
		if v.Name == name {
			return true
		}
	}
	return false
}

func (d Defaulter) WithEnv(vars []corev1.EnvVar) Defaulter {
	def, _ := d.WithNewEnv(vars)
	return def
}

func (d Defaulter) WithNewEnv(vars []corev1.EnvVar) (Defaulter, bool) {
	allNew := true
	for _, v := range vars {
		if d.envExists(v.Name) {
			allNew = false
			continue
		}
		d.base.Env = append(d.base.Env, v)
	}

	return d, allNew
}

// WithResources ensures that resource requirements are set in the container.
func (d Defaulter) WithResources(resources corev1.ResourceRequirements) Defaulter {
	if d.base.Resources.Requests == nil && d.base.Resources.Limits == nil {
		d.base.Resources = resources
	}
	return d
}

// volumeExists checks if a volume mount with the given name already exists in the Container.
func (d Defaulter) volumeMountExists(volumeMount corev1.VolumeMount) bool {
	for _, v := range d.base.VolumeMounts {
		if v.Name == volumeMount.Name || v.MountPath == volumeMount.MountPath {
			return true
		}
	}
	return false
}

func (d Defaulter) WithVolumeMounts(volumeMounts []corev1.VolumeMount) Defaulter {
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

func (d Defaulter) WithPreStopHook(handler *corev1.LifecycleHandler) Defaulter {
	if d.base.Lifecycle == nil {
		d.base.Lifecycle = &corev1.Lifecycle{}
	}

	if d.base.Lifecycle.PreStop == nil {
		// no user-provided hook, we can use our own
		d.base.Lifecycle.PreStop = handler
	}

	return d
}
