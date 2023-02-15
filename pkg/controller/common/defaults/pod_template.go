// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package defaults

import (
	"sort"

	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

// PodDownwardEnvVars returns default environment variables created from the downward API.
func PodDownwardEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: settings.EnvPodIP, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "status.podIP"},
		}},
		{Name: settings.EnvPodName, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.name"},
		}},
		{Name: settings.EnvNodeName, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "spec.nodeName"},
		}},
		{Name: settings.EnvNamespace, Value: "", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{APIVersion: "v1", FieldPath: "metadata.namespace"},
		}},
	}
}

// ExtendPodDownwardEnvVars creates a new EnvVar array with the default downward API variables prepended to given list.
func ExtendPodDownwardEnvVars(vars ...corev1.EnvVar) []corev1.EnvVar {
	podDownwardEnvVars := PodDownwardEnvVars()
	return append(podDownwardEnvVars, vars...)
}

// PodTemplateBuilder helps with building a pod template inheriting values
// from a user-provided pod template. It focuses on building a pod with
// one main Container.
type PodTemplateBuilder struct {
	PodTemplate        corev1.PodTemplateSpec
	containerName      string
	containerDefaulter container.Defaulter
}

// NewPodTemplateBuilder returns an initialized PodTemplateBuilder with some defaults.
func NewPodTemplateBuilder(base corev1.PodTemplateSpec, containerName string) *PodTemplateBuilder {
	builder := &PodTemplateBuilder{
		PodTemplate:   *base.DeepCopy(),
		containerName: containerName,
	}
	return builder.setDefaults()
}

// MainContainer retrieves the main Container from the pod template or nil if not found.
func (b *PodTemplateBuilder) MainContainer() *corev1.Container {
	for i, c := range b.PodTemplate.Spec.Containers {
		if c.Name == b.containerName {
			return &b.PodTemplate.Spec.Containers[i]
		}
	}
	return nil
}

func (b *PodTemplateBuilder) setContainerDefaulter() {
	b.containerDefaulter = container.NewDefaulter(b.MainContainer())
}

// setDefaults sets up a default Container in the pod template,
// and disables service account token auto mount.
func (b *PodTemplateBuilder) setDefaults() *PodTemplateBuilder {
	userContainer := b.MainContainer()
	if userContainer == nil {
		// create the default Container if not provided by the user
		b.PodTemplate.Spec.Containers = append(b.PodTemplate.Spec.Containers, corev1.Container{Name: b.containerName})
		b.setContainerDefaulter()
	} else {
		b.containerDefaulter = container.NewDefaulter(userContainer)
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
	b.PodTemplate.Labels = maps.MergePreservingExistingKeys(b.PodTemplate.Labels, labels)
	return b
}

// WithAnnotations sets the given annotations, but does not override those that already exist.
func (b *PodTemplateBuilder) WithAnnotations(annotations map[string]string) *PodTemplateBuilder {
	b.PodTemplate.Annotations = maps.MergePreservingExistingKeys(b.PodTemplate.Annotations, annotations)
	return b
}

// WithDockerImage sets up the Container Docker image, unless already provided.
// The default image will be used unless customImage is not empty.
func (b *PodTemplateBuilder) WithDockerImage(customImage string, defaultImage string) *PodTemplateBuilder {
	if customImage != "" {
		b.containerDefaulter.WithImage(customImage)
	} else {
		b.containerDefaulter.WithImage(defaultImage)
	}
	return b
}

// WithReadinessProbe sets up the given readiness probe, unless already provided in the template.
func (b *PodTemplateBuilder) WithReadinessProbe(readinessProbe corev1.Probe) *PodTemplateBuilder {
	b.containerDefaulter.WithReadinessProbe(&readinessProbe)
	return b
}

// WithLivenessProbe sets up the given liveness probe, unless already provided in the template.
func (b *PodTemplateBuilder) WithLivenessProbe(livenessProbe corev1.Probe) *PodTemplateBuilder {
	b.containerDefaulter.WithLivenessProbe(&livenessProbe)
	return b
}

// WithAffinity sets a default affinity, unless already provided in the template.
// An empty affinity in the spec is not overridden.
func (b *PodTemplateBuilder) WithAffinity(affinity *corev1.Affinity) *PodTemplateBuilder {
	if b.PodTemplate.Spec.Affinity == nil {
		b.PodTemplate.Spec.Affinity = affinity
	}
	return b
}

// WithPorts appends the given ports to the Container ports, unless already provided in the template.
func (b *PodTemplateBuilder) WithPorts(ports []corev1.ContainerPort) *PodTemplateBuilder {
	b.containerDefaulter.WithPorts(ports)
	return b
}

// WithCommand sets the given command to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithCommand(command []string) *PodTemplateBuilder {
	b.containerDefaulter.WithCommand(command)
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
	// order volumes by name to ensure stable pod spec comparison
	sort.SliceStable(b.PodTemplate.Spec.Volumes, func(i, j int) bool {
		return b.PodTemplate.Spec.Volumes[i].Name < b.PodTemplate.Spec.Volumes[j].Name
	})
	return b
}

// WithVolumeMounts appends the given volume mounts to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithVolumeMounts(volumeMounts ...corev1.VolumeMount) *PodTemplateBuilder {
	b.containerDefaulter.WithVolumeMounts(volumeMounts)
	return b
}

func (b *PodTemplateBuilder) WithVolumeLikes(volumeLikes ...volume.VolumeLike) *PodTemplateBuilder {
	for _, v := range volumeLikes {
		b = b.WithVolumes(v.Volume()).WithVolumeMounts(v.VolumeMount())
	}

	return b
}

// WithEnv appends the given env vars to the Container, unless already provided in the template.
func (b *PodTemplateBuilder) WithEnv(vars ...corev1.EnvVar) *PodTemplateBuilder {
	b.containerDefaulter.WithNewEnv(vars)
	return b
}

// WithNewEnv appends the given env vars to the Container, unless already provided in the template. Returns true if and
// only if the all env vars were not previously set in the Container
func (b *PodTemplateBuilder) WithNewEnv(vars ...corev1.EnvVar) (*PodTemplateBuilder, bool) {
	_, allNew := b.containerDefaulter.WithNewEnv(vars)
	return b, allNew
}

// WithTerminationGracePeriod sets the given termination grace period if not already specified in the template.
func (b *PodTemplateBuilder) WithTerminationGracePeriod(period int64) *PodTemplateBuilder {
	if b.PodTemplate.Spec.TerminationGracePeriodSeconds == nil {
		b.PodTemplate.Spec.TerminationGracePeriodSeconds = &period
	}
	return b
}

// WithContainers appends the given containers to the list of containers belonging to the pod.
// It also ensures that the base container defaulter still points to the container in the list because append()
// creates a new slice.
func (b *PodTemplateBuilder) WithContainers(containers ...corev1.Container) *PodTemplateBuilder {
	for _, c := range containers {
		found := false
		for i := range b.PodTemplate.Spec.Containers {
			podTplContainer := b.PodTemplate.Spec.Containers[i]
			if c.Name == podTplContainer.Name {
				found = true
				// inherits default values from container already defined on the pod template
				b.PodTemplate.Spec.Containers[i] = container.NewDefaulter(&podTplContainer).From(c).Container()
			}
		}
		if !found {
			b.PodTemplate.Spec.Containers = append(b.PodTemplate.Spec.Containers, c)
			b.setContainerDefaulter()
		}
	}
	return b
}

// WithInitContainerDefaults sets default values for the current init containers.
//
// Defaults:
// - If the init container contains an empty image field, it's inherited from the main container.
// - VolumeMounts from the main container are added to the init container VolumeMounts, unless they would conflict
// with a specified VolumeMount (by having the same VolumeMount.Name or VolumeMount.MountPath)
// - default environment variables
//
// This method can also be used to set some additional environment variables.
func (b *PodTemplateBuilder) WithInitContainerDefaults(additionalEnvVars ...corev1.EnvVar) *PodTemplateBuilder {
	mainContainer := b.containerDefaulter.Container()
	for i := range b.PodTemplate.Spec.InitContainers {
		b.PodTemplate.Spec.InitContainers[i] =
			container.NewDefaulter(&b.PodTemplate.Spec.InitContainers[i]).
				// Inherit image and volume mounts from main container in the Pod
				WithImage(mainContainer.Image).
				WithVolumeMounts(mainContainer.VolumeMounts).
				WithResources(mainContainer.Resources).
				WithEnv(ExtendPodDownwardEnvVars(additionalEnvVars...)).
				Container()
	}
	return b
}

// findInitContainerByName attempts to find an init container with the given name in the template
// Returns the index of the container or -1 if no init container by that name was found.
func (b *PodTemplateBuilder) findInitContainerByName(name string) int {
	for i, c := range b.PodTemplate.Spec.InitContainers {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// WithInitContainers includes the given init containers to the pod template.
//
// Ordering:
// - Provided init containers are prepended to the existing ones in the template.
// - If an init container by the same name already exists in the template, the two PodTemplates are merged, the values
// provided by the user take precedence.
func (b *PodTemplateBuilder) WithInitContainers(
	initContainers ...corev1.Container,
) *PodTemplateBuilder {
	var containers []corev1.Container

	for _, c := range initContainers {
		if index := b.findInitContainerByName(c.Name); index != -1 {
			userContainer := b.PodTemplate.Spec.InitContainers[index]

			// remove it from the podTemplate
			b.PodTemplate.Spec.InitContainers = append(
				b.PodTemplate.Spec.InitContainers[:index],
				b.PodTemplate.Spec.InitContainers[index+1:]...,
			)

			// Create a container based on what the user specified but ensure that values
			// are set if none are provided.
			containers = append(containers,
				container.
					// Set the container provided by the user as the base.
					NewDefaulter(userContainer.DeepCopy()).
					// Inherit all other values from the container built by the controller.
					From(c).
					Container())
		} else {
			containers = append(containers, c)
		}
	}

	b.PodTemplate.Spec.InitContainers = append(containers, b.PodTemplate.Spec.InitContainers...)

	return b
}

// WithResources sets up the given resource requirements if both resources limits and requests
// are nil in the main container.
// If a zero-value (empty map) for at least one of limits or request is provided, the given resource requirements
// are not applied: the user may want to use a LimitRange.
func (b *PodTemplateBuilder) WithResources(resources corev1.ResourceRequirements) *PodTemplateBuilder {
	b.containerDefaulter.WithResources(resources)
	return b
}

func (b *PodTemplateBuilder) WithPreStopHook(handler corev1.LifecycleHandler) *PodTemplateBuilder {
	b.containerDefaulter.WithPreStopHook(&handler)
	return b
}

func (b *PodTemplateBuilder) WithArgs(args ...string) *PodTemplateBuilder {
	b.containerDefaulter.WithArgs(args)
	return b
}

func (b *PodTemplateBuilder) WithServiceAccount(serviceAccount string) *PodTemplateBuilder {
	if b.PodTemplate.Spec.ServiceAccountName == "" {
		b.PodTemplate.Spec.ServiceAccountName = serviceAccount
	}
	return b
}

func (b *PodTemplateBuilder) WithHostNetwork() *PodTemplateBuilder {
	b.PodTemplate.Spec.HostNetwork = true
	return b
}

func (b *PodTemplateBuilder) WithDNSPolicy(dnsPolicy corev1.DNSPolicy) *PodTemplateBuilder {
	if b.PodTemplate.Spec.DNSPolicy == "" {
		b.PodTemplate.Spec.DNSPolicy = dnsPolicy
	}
	return b
}

func (b *PodTemplateBuilder) WithPodSecurityContext(securityContext corev1.PodSecurityContext) *PodTemplateBuilder {
	if b.PodTemplate.Spec.SecurityContext == nil {
		b.PodTemplate.Spec.SecurityContext = &securityContext
	}
	return b
}

func (b *PodTemplateBuilder) WithAutomountServiceAccountToken() *PodTemplateBuilder {
	if b.PodTemplate.Spec.AutomountServiceAccountToken == nil {
		t := true
		b.PodTemplate.Spec.AutomountServiceAccountToken = &t
	}
	return b
}
