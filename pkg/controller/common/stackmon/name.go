// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

func configVolumeName(name string, beatName string) string {
	return volume.VolumeName(name, beatName, "config")
}

func caVolumeName(assoc commonv1.Association) string {
	ref := assoc.AssociationRef()
	return volume.CAVolumeName(string(assoc.AssociationType()), ref.GetNamespace(), ref.NameOrSecretName())
}

func clientCertVolumeName(assoc commonv1.Association) string {
	ref := assoc.AssociationRef()
	// "ca-client-cert" matches the suffix that the old inline derivation produced:
	// fmt.Sprintf("%s-client-cert", caVolumeName(assoc)) → "{type}-{hash}-ca-client-cert".
	// Keeping it avoids a volume rename on existing resources when upgrading.
	return volume.VolumeNamespacedName(string(assoc.AssociationType()), ref.GetNamespace(), ref.NameOrSecretName(), "ca-client-cert")
}
