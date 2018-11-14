package initcontainer

import (
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/keystore"
	corev1 "k8s.io/api/core/v1"
)

//KeystoreInit contains keystore initialisation configuration for the init container
type KeystoreInit struct {
	Settings    []keystore.Setting
	VolumeMount corev1.VolumeMount
}

// NewPrepareFSInitContainer creates an init container to handle things such as:
// - plugins installation
// - configuration changes
// Modified directories and files are meant to be persisted for reuse in the actual ES container.
// This container does not need to be privileged.
func NewPrepareFSInitContainer(imageName string, linkedFiles LinkedFilesArray, keystoreInit KeystoreInit) (corev1.Container, error) {
	privileged := false
	initContainerRunAsUser := defaultInitContainerRunAsUser
	script, err := RenderScriptTemplate(TemplateParams{
		Plugins:          defaultInstalledPlugins,
		SharedVolumes:    SharedVolumes,
		LinkedFiles:      linkedFiles,
		KeyStoreSettings: keystoreInit.Settings,
	})
	if err != nil {
		return corev1.Container{}, err
	}
	container := corev1.Container{
		Image:           imageName,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            "prepare-fs",
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
			RunAsUser:  &initContainerRunAsUser,
		},
		Command:      []string{"bash", "-c", script},
		VolumeMounts: SharedVolumes.InitContainerVolumeMounts(),
	}

	if len(keystoreInit.Settings) > 0 {
		container.VolumeMounts = append(container.VolumeMounts, keystoreInit.VolumeMount)
	}
	return container, nil
}
