// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password/fixtures"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/keystorepassword"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileManagedKeystorePasswordSecret(t *testing.T) {
	fipsEnabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	fipsDisabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})

	esMeta := metav1.ObjectMeta{Namespace: "ns", Name: "es"}
	keystorePasswordSecretName := esv1.KeystorePasswordSecret(esMeta.Name)

	esFIPSNodeSetOnly := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{Name: "default", Config: &fipsEnabledConfig},
			},
		},
	}
	esFIPSWithUserKeystorePassword := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{
					Name:   "default",
					Config: &fipsEnabledConfig,
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									Env: []corev1.EnvVar{
										{Name: essettings.KeystorePasswordEnvVar, Value: "user-supplied"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	esFIPSDisabled := esv1.Elasticsearch{
		ObjectMeta: esMeta,
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{
				{Name: "default", Config: &fipsDisabledConfig},
			},
		},
	}
	existingKeystorePasswordSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esMeta.Namespace,
			Name:      keystorePasswordSecretName,
		},
		Data: map[string][]byte{
			keystorepassword.KeystorePasswordKey: []byte("leftover-password"),
		},
	}
	policyFIPSEnabledConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esMeta.Namespace,
			Name:      esv1.StackConfigElasticsearchConfigSecretName(esMeta.Name),
		},
		Data: map[string][]byte{
			esv1.StackConfigElasticsearchConfigKey: []byte(`{"xpack.security.fips_mode.enabled":true}`),
		},
	}

	tests := []struct {
		name               string
		es                 esv1.Elasticsearch
		extraInit          []client.Object
		esVersion          commonversion.Version
		wantReturnedSecret bool
		wantSecretInAPI    bool
	}{
		{
			name:               "below minimum version does not reconcile secret",
			es:                 esFIPSNodeSetOnly,
			esVersion:          commonversion.MinFor(9, 3, 0),
			wantReturnedSecret: false,
			wantSecretInAPI:    false,
		},
		{
			name:               "minimum version reconciles secret",
			es:                 esFIPSNodeSetOnly,
			esVersion:          esversion.KeystorePasswordMinVersion,
			wantReturnedSecret: true,
			wantSecretInAPI:    true,
		},
		{
			name:               "user-provided keystore password env skips operator secret",
			es:                 esFIPSWithUserKeystorePassword,
			esVersion:          esversion.KeystorePasswordMinVersion,
			wantReturnedSecret: false,
			wantSecretInAPI:    false,
		},
		{
			name:               "FIPS disabled does not change helper behavior",
			es:                 esFIPSDisabled,
			extraInit:          []client.Object{existingKeystorePasswordSecret},
			esVersion:          esversion.KeystorePasswordMinVersion,
			wantReturnedSecret: false,
			wantSecretInAPI:    true,
		},
		{
			name:               "FIPS enabled only via StackConfigPolicy reconciles secret",
			es:                 esFIPSDisabled,
			extraInit:          []client.Object{policyFIPSEnabledConfigSecret},
			esVersion:          esversion.KeystorePasswordMinVersion,
			wantReturnedSecret: true,
			wantSecretInAPI:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.es
			initObjs := append([]client.Object{&es}, tt.extraInit...)
			c := k8s.NewFakeClient(initObjs...)

			secret, err := reconcileManagedKeystorePasswordSecret(context.Background(), c, es, tt.esVersion, fixtures.MustTestRandomGenerator(24), metadata.Metadata{})
			require.NoError(t, err)
			if tt.wantReturnedSecret {
				require.NotNil(t, secret)
				require.Equal(t, keystorePasswordSecretName, secret.Name)
			} else {
				require.Nil(t, secret)
			}

			var stored corev1.Secret
			err = c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: keystorePasswordSecretName}, &stored)
			if tt.wantSecretInAPI {
				require.NoError(t, err)
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}
