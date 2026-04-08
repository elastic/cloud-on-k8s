// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorepassword

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	esettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	// KeystorePasswordKey is the key used in the keystore password secret.
	KeystorePasswordKey = "keystore-password"
	// VolumeName is the source Secret volume name used for init-container consumption.
	VolumeName = "keystore-password"
	// MountPath is the source Secret mount path used for init-container consumption.
	MountPath = "/mnt/elastic-internal/keystore-password"
	// PasswordFile is the mounted Secret file path read by the keystore init container.
	PasswordFile = MountPath + "/keystore-password"
)

// ReconcileKeystorePasswordSecret ensures the managed keystore password Secret
// exists with up-to-date metadata. If the Secret already exists with a
// non-empty password, the existing password is preserved; otherwise a new
// password is generated. Metadata (labels, annotations, owner references) is
// always reconciled via reconciler.ReconcileSecret.
func ReconcileKeystorePasswordSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	passwordGenerator password.RandomGenerator,
	meta metadata.Metadata,
) (*corev1.Secret, error) {
	secretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystorePasswordSecret(es.Name),
	}

	var existingSecret corev1.Secret
	if err := client.IgnoreNotFound(c.Get(ctx, secretName, &existingSecret)); err != nil {
		return nil, err
	}

	passwordBytes := existingSecret.Data[KeystorePasswordKey]
	if len(passwordBytes) == 0 {
		var err error
		length, err := passwordGenerator.Length(ctx)
		if err != nil {
			return nil, fmt.Errorf("while generating keystore password: %w", err)
		}
		// Use the larger of (user-defined length, or 14) as the length of the password to generate.
		// 14 is the minimum for a password protected keystore in Elasticsearch.
		passwordBytes, err = password.RandomBytesWithoutSymbols(max(length, 14))
		if err != nil {
			return nil, fmt.Errorf("while generating keystore password: %w", err)
		}
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   secretName.Namespace,
			Name:        secretName.Name,
			Labels:      labels.AddCredentialsLabel(maps.Merge(label.NewLabels(k8s.ExtractNamespacedName(&es)), meta.Labels)),
			Annotations: meta.Annotations,
		},
		Data: map[string][]byte{
			KeystorePasswordKey: passwordBytes,
		},
	}

	reconciled, err := reconciler.ReconcileSecret(ctx, c, expected, &es)
	if err != nil {
		return nil, fmt.Errorf("while reconciling keystore password secret: %w", err)
	}
	return &reconciled, nil
}

// DeleteKeystorePasswordSecret deletes the managed keystore password secret when
// it exists, and does not attempt a delete when not found.
func DeleteKeystorePasswordSecret(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) error {
	secretName := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystorePasswordSecret(es.Name),
	}
	var existing corev1.Secret
	if err := c.Get(ctx, secretName, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return client.IgnoreNotFound(c.Delete(ctx, &existing))
}

// InjectKeystorePassword adds the keystore password Secret volume and
// KEYSTORE_PASSWORD_FILE env var to the pod template. It modifies both the
// Elasticsearch container and the keystore init container.
func InjectKeystorePassword(builder *defaults.PodTemplateBuilder, secretName string) *defaults.PodTemplateBuilder {
	sourcePasswordVolume := corev1.Volume{
		Name: VolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: ptr.To[int32](0440),
			},
		},
	}
	sourcePasswordMount := corev1.VolumeMount{
		Name:      VolumeName,
		MountPath: MountPath,
		ReadOnly:  true,
	}

	// Configure the main Elasticsearch container:
	// - add the Secret volume to the pod and mount it on the main container
	// - set KEYSTORE_PASSWORD_FILE so docker-entrypoint reads from the Secret file
	builder = builder.
		WithVolumes(sourcePasswordVolume).
		WithVolumeMounts(sourcePasswordMount).
		WithEnv(corev1.EnvVar{
			Name:  esettings.KeystorePasswordFileEnvVar,
			Value: PasswordFile,
		})

	// Configure the keystore init container:
	// - mount the source Secret path so the init script can read PasswordFile
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

// MaybeGarbageCollectKeystorePasswordSecret deletes the managed keystore
// password secret when managed keystore passwords are not applicable for this
// resource (version < 9.4.0, FIPS disabled, or user-provided password
// override).
func MaybeGarbageCollectKeystorePasswordSecret(
	ctx context.Context,
	c k8s.Client,
	es esv1.Elasticsearch,
	esVersion commonversion.Version,
	policyElasticsearchConfig *commonsettings.CanonicalConfig,
) error {
	shouldManage, err := esettings.ShouldManageGeneratedKeystorePassword(
		ctx,
		c,
		esVersion,
		es.Namespace,
		es.Spec.NodeSets,
		policyElasticsearchConfig,
	)
	if err != nil {
		return err
	}
	if !shouldManage {
		return DeleteKeystorePasswordSecret(ctx, c, es)
	}
	return nil
}
