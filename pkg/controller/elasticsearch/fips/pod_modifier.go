// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fips

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
)

const (
	VolumeName = "fips-keystore-password"
	MountPath  = "/mnt/elastic-internal/fips-keystore-password"
	// PasswordFile is the mounted file path containing the keystore password.
	PasswordFile = MountPath + "/keystore-password"

	esKeystorePassphraseFileEnvVar = "ES_KEYSTORE_PASSPHRASE_FILE" //nolint:gosec // Environment variable name, not a secret.
)

// InjectKeystorePassword adds the FIPS keystore password Secret volume,
// volume mounts, and ES_KEYSTORE_PASSPHRASE_FILE env var to the pod template.
// It modifies both the Elasticsearch container and the keystore init container.
func InjectKeystorePassword(builder *defaults.PodTemplateBuilder, secretName string) *defaults.PodTemplateBuilder {
	fipsPasswordVolume := corev1.Volume{
		Name: VolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
			},
		},
	}
	fipsPasswordMount := corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: MountPath,
		ReadOnly:  true,
	}

	builder = builder.
		WithVolumes(fipsPasswordVolume).
		WithVolumeMounts(fipsPasswordMount).
		WithEnv(corev1.EnvVar{Name: esKeystorePassphraseFileEnvVar, Value: PasswordFile})

	for i := range builder.PodTemplate.Spec.InitContainers {
		if builder.PodTemplate.Spec.InitContainers[i].Name != keystore.InitContainerName {
			continue
		}
		builder.PodTemplate.Spec.InitContainers[i] = container.NewDefaulter(&builder.PodTemplate.Spec.InitContainers[i]).
			WithVolumeMounts([]corev1.VolumeMount{fipsPasswordMount}).
			Container()
	}

	return builder
}
