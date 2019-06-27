// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
)

const (
	DataVolumeName      = "apm-data"
	DataVolumeMountPath = "/usr/share/apm-server/data"

	SecureSettingsVolumeName      = "elastic-internal-secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic-internal/secure-settings"
)

// APMDataVolume is used to propagate the keystore file from the init container to
// the APM server running in the main container.
// Since the APM server is stateless and the keystore is created on pod start, an EmptyDir is fine here.
var DataVolume = volume.NewEmptyDirVolume(DataVolumeName, DataVolumeMountPath)
