// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package volume

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

const (
	DataVolumeName      = "kibana-data"
	DataVolumeMountPath = "/usr/share/kibana/data"
)

// KibanaDataVolume is used to propagate the keystore file from the init container to
// Kibana running in the main container.
// Since Kibana is stateless and the keystore is created on pod start, an EmptyDir is fine here.
var KibanaDataVolume = volume.NewEmptyDirVolume(DataVolumeName, DataVolumeMountPath)
