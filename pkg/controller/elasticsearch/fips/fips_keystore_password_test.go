// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fips

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileKeystorePasswordSecret(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	secretNN := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FIPSKeystorePasswordSecret(es.Name),
	}
	meta := metadata.Metadata{
		Labels:      map[string]string{"custom": "label"},
		Annotations: map[string]string{"custom-annotation": "annotation"},
	}

	tests := []struct {
		name          string
		initialSecret *corev1.Secret
		assert        func(*testing.T, *corev1.Secret)
	}{
		{
			name: "secret does not exist creates with generated password",
			assert: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				require.Len(t, secret.Data[KeystorePasswordKey], 24)
				require.NotEmpty(t, secret.Data[KeystorePasswordKey])
			},
		},
		{
			name: "existing non-empty password is reused",
			initialSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: secretNN.Namespace,
					Name:      secretNN.Name,
				},
				Data: map[string][]byte{
					KeystorePasswordKey: []byte("already-there"),
				},
			},
			assert: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				require.Equal(t, []byte("already-there"), secret.Data[KeystorePasswordKey])
				require.Empty(t, secret.OwnerReferences)
			},
		},
		{
			name: "existing empty password is regenerated",
			initialSecret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: secretNN.Namespace,
					Name:      secretNN.Name,
				},
				Data: map[string][]byte{
					KeystorePasswordKey: {},
				},
			},
			assert: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				require.Len(t, secret.Data[KeystorePasswordKey], 24)
				require.NotEmpty(t, secret.Data[KeystorePasswordKey])
			},
		},
		{
			name: "owner reference and labels are set",
			assert: func(t *testing.T, secret *corev1.Secret) {
				t.Helper()
				require.Len(t, secret.OwnerReferences, 1)
				require.Equal(t, es.Name, secret.OwnerReferences[0].Name)
				require.Equal(t, label.Type, secret.Labels[commonv1.TypeLabelName])
				require.Equal(t, es.Name, secret.Labels[label.ClusterNameLabelName])
				require.Equal(t, "label", secret.Labels["custom"])
				require.Equal(t, "annotation", secret.Annotations["custom-annotation"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []client.Object{&es}
			if tt.initialSecret != nil {
				objects = append(objects, tt.initialSecret)
			}
			c := k8s.NewFakeClient(objects...)

			reconciled, err := ReconcileKeystorePasswordSecret(context.Background(), c, es, meta)
			require.NoError(t, err)
			require.NotNil(t, reconciled)

			var secret corev1.Secret
			err = c.Get(context.Background(), secretNN, &secret)
			require.NoError(t, err)
			require.Equal(t, reconciled.Data[KeystorePasswordKey], secret.Data[KeystorePasswordKey])

			tt.assert(t, &secret)
		})
	}
}

func TestDeleteKeystorePasswordSecretIfExists(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	secretNN := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.FIPSKeystorePasswordSecret(es.Name),
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNN.Namespace,
			Name:      secretNN.Name,
		},
		Data: map[string][]byte{KeystorePasswordKey: []byte("existing")},
	}
	c := k8s.NewFakeClient(secret)

	require.NoError(t, DeleteKeystorePasswordSecretIfExists(context.Background(), c, es))

	var deleted corev1.Secret
	err := c.Get(context.Background(), secretNN, &deleted)
	require.True(t, apierrors.IsNotFound(err))

	require.NoError(t, DeleteKeystorePasswordSecretIfExists(context.Background(), c, es))
}
