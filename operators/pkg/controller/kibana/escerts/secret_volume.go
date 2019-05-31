// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package escerts

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

var eSCertsVolumeMountPath = "/usr/share/kibana/config/elasticsearch-certs"

// SecretVolume returns a SecretVolume to hold the Elasticsearch certs for the given Kibana resource.
func SecretVolume(kb v1alpha1.Kibana) volume.SecretVolume {
	// TODO: this is a little ugly as it reaches into the ES controller bits
	return volume.NewSecretVolumeWithMountPath(
		kb.Spec.Elasticsearch.CaCertSecret,
		"elasticsearch-certs",
		eSCertsVolumeMountPath,
	)
}
