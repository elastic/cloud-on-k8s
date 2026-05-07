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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/securitycontext"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
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
	policyConfig, err := nodespec.GetPolicyConfig(ctx, client, es)
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
