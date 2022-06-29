// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackmon

import (
	"crypto/sha256"
	"fmt"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
)

const maxVolumeNameLength = 63

var VolumeNamer = name.Namer{
	MaxSuffixLength: name.MaxSuffixLength,
	MaxNameLength:   maxVolumeNameLength,
}

func configVolumeName(name string, beatName string) string {
	return VolumeNamer.Suffix(name, beatName, "config")
}

func caVolumeName(assoc commonv1.Association) string {
	nsn := assoc.AssociationRef().Namespace + assoc.AssociationRef().NameOrSecretName()
	nsnHash := fmt.Sprintf("%x", sha256.Sum256([]byte(nsn)))[0:6]
	return VolumeNamer.Suffix(string(assoc.AssociationType()), nsnHash, "ca")
}
