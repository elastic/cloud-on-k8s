// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fips

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// KeystorePasswordKey is the key used in the FIPS keystore password secret.
	KeystorePasswordKey = "keystore-password"

	SourceVolumeName = "fips-keystore-password"
	SourceMountPath  = "/mnt/elastic-internal/fips-keystore-password"
	// SourcePasswordFile is the mounted Secret file path read by the keystore init container.
	SourcePasswordFile = SourceMountPath + "/keystore-password"

	keystorePasswordFileEnvVar = "KEYSTORE_PASSWORD_FILE"

	generatedPasswordLength = 24
)

const (
	// VolumeName is the source Secret volume name used for init-container consumption.
	VolumeName = SourceVolumeName
	// MountPath is the source Secret mount path used for init-container consumption.
	MountPath = SourceMountPath
	// PasswordFile is the source Secret password file path used by the init script.
	PasswordFile = SourcePasswordFile
)

// ReconcileKeystorePasswordSecret ensures the FIPS keystore password Secret
// exists with up-to-date metadata. If the Secret already exists with a
// non-empty password, the existing password is preserved; otherwise a new
// password is generated. Metadata (labels, annotations, owner references) is
// always reconciled via reconciler.ReconcileSecret.
func ReconcileKeystorePasswordSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	meta metadata.Metadata,
) (*corev1.Secret, error) {
	secretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FIPSKeystorePasswordSecret(es.Name),
	}

	var existingSecret corev1.Secret
	if err := client.IgnoreNotFound(c.Get(ctx, secretName, &existingSecret)); err != nil {
		return nil, err
	}

	passwordBytes := existingSecret.Data[KeystorePasswordKey]
	if len(passwordBytes) == 0 {
		var err error
		passwordBytes, err = password.RandomBytesWithoutSymbols(generatedPasswordLength)
		if err != nil {
			return nil, fmt.Errorf("while generating fips keystore password: %w", err)
		}
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   secretName.Namespace,
			Name:        secretName.Name,
			Labels:      maps.Merge(label.NewLabels(k8s.ExtractNamespacedName(&es)), meta.Labels),
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			KeystorePasswordKey: passwordBytes,
		},
	}

	reconciled, err := reconciler.ReconcileSecret(ctx, c, expected, &es)
	if err != nil {
		return nil, fmt.Errorf("while reconciling fips keystore password secret: %w", err)
	}
	return &reconciled, nil
}

// DeleteKeystorePasswordSecret deletes the FIPS keystore password
// secret, if present.
func DeleteKeystorePasswordSecret(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	return client.IgnoreNotFound(c.Delete(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      esv1.FIPSKeystorePasswordSecret(es.Name),
		},
	}))
}

// InjectKeystorePassword adds the FIPS keystore password Secret volume and
// KEYSTORE_PASSWORD_FILE env var to the pod template. It modifies both the
// Elasticsearch container and the keystore init container.
func InjectKeystorePassword(builder *defaults.PodTemplateBuilder, secretName string) *defaults.PodTemplateBuilder {
	sourcePasswordVolume := corev1.Volume{
		Name: SourceVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To[int32](0400),
			},
		},
	}
	sourcePasswordMount := corev1.VolumeMount{
		Name:      SourceVolumeName,
		MountPath: SourceMountPath,
		ReadOnly:  true,
	}

	// Main Elasticsearch container wiring:
	// - add the Secret volume to the pod and mount it on the main container
	// - set KEYSTORE_PASSWORD_FILE so docker-entrypoint reads from the Secret file
	builder = builder.
		WithVolumes(sourcePasswordVolume).
		WithVolumeMounts(sourcePasswordMount).
		WithEnv(corev1.EnvVar{
			Name:  keystorePasswordFileEnvVar,
			Value: SourcePasswordFile,
		})

	// Keystore init container wiring:
	// - mount the source Secret path so the init script can read SourcePasswordFile
	for i := range builder.PodTemplate.Spec.InitContainers {
		if builder.PodTemplate.Spec.InitContainers[i].Name != keystore.InitContainerName {
			continue
		}
		builder.PodTemplate.Spec.InitContainers[i] = container.NewDefaulter(&builder.PodTemplate.Spec.InitContainers[i]).
			WithVolumeMounts([]corev1.VolumeMount{sourcePasswordMount}).
			Container()
	}

	return builder
}
