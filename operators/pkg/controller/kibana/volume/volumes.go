package volume

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
)

const (
	DataVolumeName      = "kibana-data"
	DataVolumeMountPath = "/usr/share/kibana/data"

	SecureSettingsVolumeName      = "elastic-internal-secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic-internal/secure-settings"
)

// KibanaDataVolume is used to propagate the keystore file from the init container to
// Kibana running in the main container.
// Since Kibana is stateless and the keystore is created on pod start, an EmptyDir is fine here.
var KibanaDataVolume = volume.NewEmptyDirVolume(DataVolumeName, DataVolumeMountPath)
