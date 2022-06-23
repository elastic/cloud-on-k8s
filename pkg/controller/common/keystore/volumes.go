// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystore

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
)

const (
	DataVolumeNamePattern = "%s-data"

	SecureSettingsVolumeName      = "elastic-internal-secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic-internal/secure-settings"
)

// dataVolumeName returns the volume name in which the keystore will be stored.
func dataVolumeName(prefix string) string {
	return fmt.Sprintf(DataVolumeNamePattern, prefix)
}

// DataVolume returns the volume used to propagate the keystore file from the init container to
// the server running in the main container.
// Since the APM server or Kibana are stateless and the keystore is created on pod start, an EmptyDir is fine here.
func DataVolume(prefix string, dataVolumePath string) volume.EmptyDirVolume {
	return volume.NewEmptyDirVolume(dataVolumeName(prefix), dataVolumePath)
}
