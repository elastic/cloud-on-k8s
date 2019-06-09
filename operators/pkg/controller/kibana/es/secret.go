// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package es

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

var eSCertsVolumeMountPath = "/usr/share/kibana/config/elasticsearch-certs"

// CaCertSecretVolume returns a SecretVolume to hold the Elasticsearch CA certs for the given Kibana resource.
func CaCertSecretVolume(kb v1alpha1.Kibana) volume.SecretVolume {
	// TODO: this is a little ugly as it reaches into the ES controller bits
	return volume.NewSecretVolumeWithMountPath(
		kb.Spec.Elasticsearch.CaCertSecret,
		"elasticsearch-certs",
		eSCertsVolumeMountPath,
	)
}

// GetAuthSecret returns the Elasticsearch auth secret for the given Kibana resource.
func GetAuthSecret(client k8s.Client, kb v1alpha1.Kibana) (*corev1.Secret, error) {
	esAuthSecret := types.NamespacedName{
		Name:      kb.Spec.Elasticsearch.Auth.SecretKeyRef.Name,
		Namespace: kb.Namespace,
	}
	var secret corev1.Secret
	err := client.Get(esAuthSecret, &secret)
	if err != nil {
		return nil, err
	}
	return &secret, nil
}
