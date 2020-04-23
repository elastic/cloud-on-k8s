// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
)

var eSCertsVolumeMountPath = "/usr/share/kibana/config/elasticsearch-certs"

// CaCertSecretVolume returns a SecretVolume to hold the Elasticsearch CA certs for the given Kibana resource.
func CaCertSecretVolume(kb kbv1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		kb.AssociationConf().GetCASecretName(),
		"elasticsearch-certs",
		eSCertsVolumeMountPath,
	)
}

