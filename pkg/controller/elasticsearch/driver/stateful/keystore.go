// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver/shared"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/securitycontext"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/stackconfig"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	remotekeystore "github.com/elastic/cloud-on-k8s/v3/pkg/controller/remotecluster/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// reconcileKeystore reconciles the keystore init container, associated volumes and,
// when applicable, the managed keystore password Secret for stateful Elasticsearch.
// Stateless clusters do not use the init container keystore (they deliver secure
// settings via cluster_secrets in file-based settings).
func (d *Driver) reconcileKeystore(ctx context.Context, meta metadata.Metadata) (*keystore.Resources, error) {
	keystoreParams := initcontainer.KeystoreParams
	keystoreSecurityContext := securitycontext.For(d.Version, true)
	keystoreParams.SecurityContext = &keystoreSecurityContext

	keystorePasswordSecret, err := reconcileManagedKeystorePasswordSecret(ctx, d.Client, d.ES, d.Version, d.OperatorParameters.PasswordGenerator, meta)
	if err != nil {
		return nil, err
	}
	if keystorePasswordSecret != nil {
		keystoreParams.KeystorePasswordPath = keystorepassword.PasswordFile
		keystorepassword.ApplyPasswordProtectedKeystoreScript(&keystoreParams)
	}

	remoteClusterAPIKeys, err := remotekeystore.APIKeySecretSource(ctx, &d.ES, d.Client)
	if err != nil {
		return nil, err
	}
	keystoreResources, err := keystore.ReconcileResources(
		ctx,
		d,
		&d.ES,
		esv1.ESNamer,
		meta,
		d.OperatorParameters.OperatorNamespace,
		keystoreParams,
		remoteClusterAPIKeys...,
	)
	if err != nil {
		return nil, err
	}
	if keystoreResources != nil && keystorePasswordSecret != nil {
		keystoreResources.KeystorePasswordSecretName = keystorePasswordSecret.Name
		keystoreResources.KeystorePasswordSecretHash = hash.HashObject(keystorePasswordSecret.Data)
	}
	return keystoreResources, nil
}

// reconcileManagedKeystorePasswordSecret reconciles the managed keystore
// password secret when managed keystore passwords are applicable (version
// threshold met, FIPS enabled, and no user-provided password override).
func reconcileManagedKeystorePasswordSecret(
	ctx context.Context,
	client k8s.Client,
	es esv1.Elasticsearch,
	esVersion version.Version,
	passwordGenerator password.RandomGenerator,
	meta metadata.Metadata,
) (*corev1.Secret, error) {
	policyConfig, err := stackconfig.GetPolicyConfig(ctx, client, es)
	if err != nil {
		return nil, err
	}
	shouldManage, err := settings.ShouldManageGeneratedKeystorePassword(
		ctx,
		client,
		esVersion,
		es.Namespace,
		es.Spec.NodeSets,
		policyConfig.ElasticsearchConfig,
	)
	if err != nil {
		return nil, err
	}
	if !shouldManage {
		return nil, nil
	}
	return keystorepassword.ReconcileKeystorePasswordSecret(ctx, client, es, passwordGenerator, meta)
}

// reconcileSecureSettings routes secure-settings reconciliation based on the ES version and
// opt-in annotation. For ES >= 9.5 with the file-based annotation, settings are delivered
// via cluster_secrets in the file-based settings JSON (no init container, hot-reload capable).
// For all other cases the standard keystore init container path is used.
func (d *Driver) reconcileSecureSettings(ctx context.Context, meta metadata.Metadata) (*keystore.Resources, error) {
	if d.Version.GTE(esversion.FileBasedSecureSettingsMinVersion) && d.ES.HasFileBasedSecureSettingsAnnotation() {
		clusterSecrets, err := shared.BuildClusterSecrets(ctx, d.Client, d.Recorder(), d.DynamicWatches(), d.ES, d.OperatorParameters.OperatorNamespace)
		if err != nil {
			return nil, err
		}
		if err := filesettings.ReconcileClusterSecrets(ctx, d.Client, d.ES, clusterSecrets); err != nil {
			return nil, err
		}
		return nil, keystore.DeleteSecureSettingsSecret(ctx, d.Client, esv1.ESNamer, &d.ES)
	}
	// Keystore path
	if d.Version.GTE(esversion.FileBasedSecureSettingsMinVersion) {
		// Clear any stale cluster_secrets
		if err := filesettings.ReconcileClusterSecrets(ctx, d.Client, d.ES, nil); err != nil {
			return nil, err
		}
	}
	return d.reconcileKeystore(ctx, meta)
}
