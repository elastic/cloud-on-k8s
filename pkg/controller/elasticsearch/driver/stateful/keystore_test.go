// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stateful

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password/fixtures"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
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
		Namespace: esMeta.Namespace,
		Name:      keystorePasswordSecretName,
		Data: map[string][]byte{
			keystorepassword.KeystorePasswordKey: []byte("leftover-password"),
		},
	}
	policyFIPSEnabledConfigSecret := &corev1.Secret{
		Namespace: esMeta.Namespace,
		Name:      esv1.StackConfigElasticsearchConfigSecretName(esMeta.Name),
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
		{
			// ES >= 9.5 without the opt-in annotation still uses the keystore path; FIPS applies.
			// The annotation-based routing to the file-based path is handled in driver.go, not here.
			name:               "ES >= 9.5 without annotation with FIPS enabled reconciles password secret",
			es:                 esFIPSNodeSetOnly,
			esVersion:          esversion.FileBasedSecureSettingsMinVersion,
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

func TestReconcileSecureSettings(t *testing.T) {
	const ns = "ns"
	const esName = "es"
	const secureSettingsSecretName = "my-secure-settings"

	secureSettingsSecret := &corev1.Secret{
		Namespace: ns, Name: secureSettingsSecretName,
		Data: map[string][]byte{"my-key": []byte("my-value")},
	}

	esWithAnnotation := esv1.Elasticsearch{
		Namespace: ns, Name: esName,
		Annotations: map[string]string{esv1.FileBasedSecureSettingsAnnotation: "true"},
		Spec: esv1.ElasticsearchSpec{
			SecureSettings: []commonv1.SecretSource{{SecretName: secureSettingsSecretName}},
		},
	}
	esNoAnnotation := esv1.Elasticsearch{
		Namespace: ns, Name: esName,
		Spec: esv1.ElasticsearchSpec{
			SecureSettings: []commonv1.SecretSource{{SecretName: secureSettingsSecretName}},
		},
	}

	// staleFileSettingsSecret simulates a cluster that previously used file-based delivery
	// (annotation path) and left cluster_secrets in the file-settings Secret.
	staleClusterSecretsCfg := commonv1.NewConfig(map[string]any{"stale-key": "stale-value"})
	staleSettings := filesettings.NewEmptySettings(1)
	staleSettings.State.ClusterSecrets = &staleClusterSecretsCfg
	staleSettingsBytes, err := json.Marshal(staleSettings)
	require.NoError(t, err)
	staleFileSettingsSecret := &corev1.Secret{
		Namespace: ns, Name: esv1.FileSettingsSecretName(esName),
		Data: map[string][]byte{filesettings.SettingsSecretKey: staleSettingsBytes},
	}

	tests := []struct {
		name                string
		es                  esv1.Elasticsearch
		extraObjs           []client.Object
		version             commonversion.Version
		fileSettingsRequeue bool
		wantNilResources    bool
		checkFn             func(*testing.T, k8s.Client)
	}{
		{
			name:                "file-based annotation path: returns nil when fileSettingsRequeue",
			es:                  esWithAnnotation,
			version:             esversion.FileBasedSecureSettingsMinVersion,
			fileSettingsRequeue: true,
			wantNilResources:    true,
		},
		{
			name:                "file-based annotation path: returns nil when not fileSettingsRequeue",
			es:                  esWithAnnotation,
			extraObjs:           []client.Object{secureSettingsSecret},
			version:             esversion.FileBasedSecureSettingsMinVersion,
			fileSettingsRequeue: false,
			wantNilResources:    true,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var secret corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: ns, Name: esv1.FileSettingsSecretName(esName)}, &secret))
				var settings filesettings.Settings
				require.NoError(t, json.Unmarshal(secret.Data[filesettings.SettingsSecretKey], &settings))
				require.NotNil(t, settings.State.ClusterSecrets, "cluster_secrets must be written by the annotation path")
				stringSecrets, ok := settings.State.ClusterSecrets.Data["string_secrets"]
				require.True(t, ok, "cluster_secrets must contain string_secrets")
				require.Equal(t, map[string]any{"my-key": "my-value"}, stringSecrets)
			},
		},
		{
			name:                "keystore path: runs reconcileKeystore even when fileSettingsRequeue",
			es:                  esNoAnnotation,
			extraObjs:           []client.Object{secureSettingsSecret},
			version:             esversion.FileBasedSecureSettingsMinVersion,
			fileSettingsRequeue: true,
			wantNilResources:    false,
		},
		{
			name:             "keystore path: clears stale cluster_secrets from file-settings Secret",
			es:               esNoAnnotation,
			extraObjs:        []client.Object{secureSettingsSecret, staleFileSettingsSecret},
			version:          esversion.FileBasedSecureSettingsMinVersion,
			wantNilResources: false,
			checkFn: func(t *testing.T, c k8s.Client) {
				t.Helper()
				var secret corev1.Secret
				require.NoError(t, c.Get(t.Context(), types.NamespacedName{Namespace: ns, Name: esv1.FileSettingsSecretName(esName)}, &secret))
				var settings filesettings.Settings
				require.NoError(t, json.Unmarshal(secret.Data[filesettings.SettingsSecretKey], &settings))
				require.Nil(t, settings.State.ClusterSecrets)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := tt.es
			initObjs := append([]client.Object{&es}, tt.extraObjs...)
			c := k8s.NewFakeClient(initObjs...)

			d := &Driver{
				Parameters: driver.Parameters{
					ES:             es,
					Version:        tt.version,
					Client:         c,
					Recorder:       toolsevents.NewFakeRecorder(10),
					DynamicWatches: watches.NewDynamicWatches(),
					OperatorParameters: operator.Parameters{
						PasswordGenerator: fixtures.MustTestRandomGenerator(24),
					},
				},
			}

			resources, err := d.reconcileSecureSettings(t.Context(), metadata.Metadata{}, tt.fileSettingsRequeue)
			require.NoError(t, err)
			if tt.wantNilResources {
				require.Nil(t, resources)
			} else {
				require.NotNil(t, resources)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, c)
			}
		})
	}
}
