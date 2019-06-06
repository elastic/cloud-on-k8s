// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
)

// Constants to use for the config files in a Kibana pod.
const (
	VolumeName      = "config"
	VolumeMountPath = "/usr/share/kibana/" + VolumeName
)

// SecretVolume returns a SecretVolume to hold the Kibana config of the given Kibana resource.
func SecretVolume(kb v1alpha1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		SecretName(kb),
		VolumeName,
		VolumeMountPath,
	)
}

// SecretName is the name of the secret that holds the Kibana config for the given Kibana resource.
func SecretName(kb v1alpha1.Kibana) string {
	return kb.Name + "-kb-" + VolumeName
}
