// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

// keystoreInConfigDirVersion is the version in which the keystore is no longer stored in the data directory but in the config one.
var keystoreInConfigDirVersion = version.From(7, 9, 0)

// newInitContainersParameters is used to generate the init container that will load the secure settings into a keystore
func newInitContainersParameters(kb *kbv1.Kibana) (keystore.InitContainerParameters, error) {
	parameters := keystore.InitContainerParameters{
		KeystoreCreateCommand:         "/usr/share/kibana/bin/kibana-keystore create",
		KeystoreAddCommand:            `/usr/share/kibana/bin/kibana-keystore add "$key" --stdin < "$filename"`,
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		KeystoreVolumePath:            DataVolumeMountPath,
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
	}

	kbVersion, err := version.Parse(kb.Spec.Version)
	if err != nil {
		return parameters, err
	}

	if kbVersion.GTE(keystoreInConfigDirVersion) {
		parameters.KeystoreVolumePath = ConfigSharedVolume.ContainerMountPath
	}

	return parameters, nil
}
