// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/name"
	kbvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/volume"
)

// HTTPCertSecretVolume returns a SecretVolume to hold the HTTP certs for the given Kibana resource.
func HTTPCertSecretVolume(kb v1alpha1.Kibana) volume.SecretVolume {
	return volume.NewSecretVolumeWithMountPath(
		certificates.HTTPCertsInternalSecretName(name.KBNamer, kb.Name),
		kbvolume.HTTPCertificatesSecretVolumeName,
		kbvolume.HTTPCertificatesSecretVolumeMountPath,
	)
}
