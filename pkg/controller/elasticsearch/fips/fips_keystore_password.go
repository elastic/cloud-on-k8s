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
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
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

	generatedPasswordLength = 24
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
