// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	DataVolumeNamePattern = "%s-data"
	//DataVolumeMountPath = "/usr/share/apm-server/data"

	SecureSettingsVolumeName      = "elastic-internal-secure-settings"
	SecureSettingsVolumeMountPath = "/mnt/elastic-internal/secure-settings"
)

// DataVolumeName returns the volume name in which the keystore will be stored.
func DataVolumeName(object runtime.Object) string {
	return strings.ToLower(fmt.Sprintf(DataVolumeNamePattern, object.GetObjectKind().GroupVersionKind().Kind))
}

// DataVolume returns the volume used to propagate the keystore file from the init container to
// the server running in the main container.
// Since the APM server or Kibana are stateless and the keystore is created on pod start, an EmptyDir is fine here.
func DataVolume(object runtime.Object, dataVolumePath string) volume.EmptyDirVolume {
	return volume.NewEmptyDirVolume(DataVolumeName(object), dataVolumePath)
}
