// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package securesettings

import (
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("secure-settings")

type SecureSettings struct {
	Volume        corev1.Volume
	InitContainer corev1.Container
	Version       string
}

// Resources optionally returns a volume and init container to include in pods,
// in order to create a keystore from secure settings referenced in the spec of the custom resource.
func Resources(
	c k8s.Client,
	recorder record.EventRecorder,
	watches watches.DynamicWatches,
	keystoreBinaryName string,
	owner runtime.Object,
	namespacedName types.NamespacedName,
	secureSettingsSecretsRef *commonv1alpha1.SecretRef,
	secureSettingsVolumeName string,
	secureSettingsVolumeMountPath string,
	dataVolumeMount corev1.VolumeMount,
) (SecureSettings, error) {
	// setup a volume from the user-provided secure settings secret
	secretVolume, version, err := secureSettingsVolume(
		c,
		recorder,
		watches,
		owner,
		namespacedName,
		secureSettingsVolumeName,
		secureSettingsVolumeMountPath,
		secureSettingsSecretsRef,
	)
	if err != nil {
		return SecureSettings{}, err
	}
	if secretVolume == nil {
		// nothing to do
		return SecureSettings{}, nil
	}

	// build an init container to create the keystore from the secure settings volume
	initContainer := initContainer(
		*secretVolume,
		secureSettingsVolumeMountPath,
		dataVolumeMount,
		keystoreBinaryName,
	)

	return SecureSettings{
		secretVolume.Volume(),
		initContainer,
		version,
	}, nil
}
