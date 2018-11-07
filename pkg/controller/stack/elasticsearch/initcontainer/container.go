package initcontainer

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// defaultInitContainerPrivileged determines if the init container should be privileged
	defaultInitContainerPrivileged bool = true
	// defaultInitContainerRunAsUser is the user id the init container should run as
	defaultInitContainerRunAsUser int64 = 0
)

// NewInitContainer creates an init container to handle things such as:
// - tweak OS settings
// - install extra plugins
func NewInitContainer(imageName string, setVMMaxMapCount bool) (corev1.Container, error) {
	initContainerPrivileged := defaultInitContainerPrivileged
	initContainerRunAsUser := defaultInitContainerRunAsUser
	script, err := RenderScriptTemplate(TemplateParams{
		SetVMMaxMapCount: setVMMaxMapCount,
		Plugins:          defaultInstalledPlugins,
		SharedVolumes:    SharedVolumes,
	})
	if err != nil {
		return corev1.Container{}, err
	}
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            "init-es",
		SecurityContext: &corev1.SecurityContext{
			Privileged: &initContainerPrivileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Command:      []string{"bash", "-c", script},
		VolumeMounts: SharedVolumes.InitContainerVolumeMounts(),
	}
	return container, nil
}
