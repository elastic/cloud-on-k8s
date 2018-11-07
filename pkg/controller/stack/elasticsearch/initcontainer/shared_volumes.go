package initcontainer

import corev1 "k8s.io/api/core/v1"

// Volumes that are shared between the init container and the ES container
var (
	SharedVolumes = SharedVolumeArray{
		Array: []SharedVolume{
			// Contains configuration (elasticsearch.yml) and plugins configuration subdirs
			SharedVolume{
				Name: "config-volume",
				InitContainerMountPath: "/volume/config",
				EsContainerMountPath:   "/usr/share/elasticsearch/config",
			},
			// Contains plugins data
			SharedVolume{
				Name: "plugins-volume",
				InitContainerMountPath: "/volume/plugins",
				EsContainerMountPath:   "/usr/share/elasticsearch/plugins",
			},
			// Plugins may have binaries installed in /bin
			SharedVolume{
				Name: "bin-volume",
				InitContainerMountPath: "/volume/bin",
				EsContainerMountPath:   "/usr/share/elasticsearch/bin",
			},
		},
	}
)

// SharedVolume between the init container and the ES container
type SharedVolume struct {
	Name                   string // Volume name
	InitContainerMountPath string // Mount path in the init container
	EsContainerMountPath   string // Mount path in the Elasticsearch container
}

// SharedVolumes represents a list of SharedVolume
type SharedVolumeArray struct {
	Array []SharedVolume
}

func (v SharedVolumeArray) InitContainerVolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, len(v.Array))
	for i, v := range v.Array {
		mounts[i] = corev1.VolumeMount{
			MountPath: v.InitContainerMountPath,
			Name:      v.Name,
		}
	}
	return mounts
}

func (v SharedVolumeArray) EsContainerVolumeMounts() []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, len(v.Array))
	for i, v := range v.Array {
		mounts[i] = corev1.VolumeMount{
			MountPath: v.EsContainerMountPath,
			Name:      v.Name,
		}
	}
	return mounts
}

func (v SharedVolumeArray) Volumes() []corev1.Volume {
	volumes := make([]corev1.Volume, len(v.Array))
	for i, v := range v.Array {
		volumes[i] = corev1.Volume{
			Name: v.Name,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
	}
	return volumes
}
