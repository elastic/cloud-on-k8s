// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

const (
	KeystoreBinPath = "/usr/share/elasticsearch/bin/elasticsearch-keystore"
)

// KeystoreParams is used to generate the init container that will load the secure settings into a keystore.
var KeystoreParams = keystore.InitContainerParameters{
	KeystoreCreateCommand:         KeystoreBinPath + " create",
	KeystoreAddCommand:            KeystoreBinPath + ` add-file "$key" "$filename"`,
	SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
	DataVolumePath:                esvolume.ElasticsearchDataMountPath,
}
