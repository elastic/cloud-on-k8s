// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package keystorepassword

import (
	"bytes"
	"context"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	commonpassword "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/password/fixtures"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	commonversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	esversion "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func renderCustomScript(t *testing.T, parameters keystore.InitContainerParameters) string {
	t.Helper()
	tpl, err := template.New("").Parse(parameters.CustomScript)
	require.NoError(t, err)

	var out bytes.Buffer
	err = tpl.Execute(&out, parameters)
	require.NoError(t, err)
	return out.String()
}

func TestReconcileKeystorePasswordSecret(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	secretNN := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystorePasswordSecret(es.Name),
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
			name: "existing non-empty password is reused and metadata is reconciled",
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
				require.Len(t, secret.OwnerReferences, 1)
				require.Equal(t, es.Name, secret.OwnerReferences[0].Name)
				require.Equal(t, label.Type, secret.Labels[commonv1.TypeLabelName])
				require.Equal(t, es.Name, secret.Labels[label.ClusterNameLabelName])
				require.Equal(t, "label", secret.Labels["custom"])
				require.Equal(t, "annotation", secret.Annotations["custom-annotation"])
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

			reconciled, err := ReconcileKeystorePasswordSecret(context.Background(), c, es, fixtures.MustTestRandomGenerator(24), meta)
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

func TestDeleteKeystorePasswordSecret(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	secretNN := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystorePasswordSecret(es.Name),
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: secretNN.Namespace,
			Name:      secretNN.Name,
		},
		Data: map[string][]byte{KeystorePasswordKey: []byte("existing")},
	}
	c := k8s.NewFakeClient(secret)

	require.NoError(t, DeleteKeystorePasswordSecret(context.Background(), c, es))

	var deleted corev1.Secret
	err := c.Get(context.Background(), secretNN, &deleted)
	require.True(t, apierrors.IsNotFound(err))

	require.NoError(t, DeleteKeystorePasswordSecret(context.Background(), c, es))
}

func TestMaybeGarbageCollectKeystorePasswordSecret(t *testing.T) {
	fipsEnabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	fipsDisabledConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})

	tests := []struct {
		name           string
		es             esv1.Elasticsearch
		esVersion      commonversion.Version
		policyConfig   *commonsettings.CanonicalConfig
		wantSecretLeft bool
	}{
		{
			name: "below minimum version deletes secret",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "default", Config: &fipsEnabledConfig},
					},
				},
			},
			esVersion:      commonversion.MinFor(9, 3, 0),
			wantSecretLeft: false,
		},
		{
			name: "fips enabled and no override keeps secret",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "default", Config: &fipsEnabledConfig},
					},
				},
			},
			esVersion:      esversion.KeystorePasswordMinVersion,
			wantSecretLeft: true,
		},
		{
			name: "fips disabled deletes secret",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "default", Config: &fipsDisabledConfig},
					},
				},
			},
			esVersion:      esversion.KeystorePasswordMinVersion,
			wantSecretLeft: false,
		},
		{
			name: "user override deletes secret",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
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
												{Name: "KEYSTORE_PASSWORD_FILE", Value: "/tmp/user-managed"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			esVersion:      esversion.KeystorePasswordMinVersion,
			wantSecretLeft: false,
		},
		{
			name: "policy-only fips keeps secret",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
				Spec: esv1.ElasticsearchSpec{
					NodeSets: []esv1.NodeSet{
						{Name: "default", Config: &fipsDisabledConfig},
					},
				},
			},
			esVersion:      esversion.KeystorePasswordMinVersion,
			policyConfig:   commonsettings.MustCanonicalConfig(map[string]any{"xpack.security.fips_mode.enabled": true}),
			wantSecretLeft: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretName := types.NamespacedName{
				Namespace: tt.es.Namespace,
				Name:      esv1.KeystorePasswordSecret(tt.es.Name),
			}
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: secretName.Namespace,
					Name:      secretName.Name,
				},
				Data: map[string][]byte{KeystorePasswordKey: []byte("existing")},
			}
			c := k8s.NewFakeClient(&tt.es, existingSecret)

			err := MaybeGarbageCollectKeystorePasswordSecret(context.Background(), c, tt.es, tt.esVersion, tt.policyConfig)
			require.NoError(t, err)

			var secret corev1.Secret
			err = c.Get(context.Background(), secretName, &secret)
			if tt.wantSecretLeft {
				require.NoError(t, err)
				return
			}
			require.True(t, apierrors.IsNotFound(err))
		})
	}
}

func TestReconcileKeystorePasswordSecret_UsesDefaultLengthWhenUseLengthDisabled(t *testing.T) {
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	secretNN := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      esv1.KeystorePasswordSecret(es.Name),
	}
	c := k8s.NewFakeClient(&es)
	meta := metadata.Metadata{}

	generator, err := commonpassword.NewRandomPasswordGenerator(72, func(context.Context) (bool, error) { return false, nil })
	require.NoError(t, err)

	reconciled, err := ReconcileKeystorePasswordSecret(context.Background(), c, es, generator, meta)
	require.NoError(t, err)
	require.NotNil(t, reconciled)
	require.Len(t, reconciled.Data[KeystorePasswordKey], 24)

	var secret corev1.Secret
	err = c.Get(context.Background(), secretNN, &secret)
	require.NoError(t, err)
	require.Len(t, secret.Data[KeystorePasswordKey], 24)
}

func TestInjectKeystorePassword(t *testing.T) {
	tests := []struct {
		name        string
		podTemplate corev1.PodTemplateSpec
	}{
		{
			name: "inject into empty template",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: keystore.InitContainerName},
					},
				},
			},
		},
		{
			name: "preserve existing volumes and mounts",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "existing-volume",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "existing-volume", MountPath: "/existing"},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name: keystore.InitContainerName,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "existing-volume", MountPath: "/existing"},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := defaults.NewPodTemplateBuilder(tt.podTemplate, esv1.ElasticsearchContainerName)
			builder = InjectKeystorePassword(builder, "es-es-keystore-password")

			var sourceVolume *corev1.Volume
			for i := range builder.PodTemplate.Spec.Volumes {
				if builder.PodTemplate.Spec.Volumes[i].Name == VolumeName {
					sourceVolume = &builder.PodTemplate.Spec.Volumes[i]
				}
			}
			require.NotNil(t, sourceVolume)
			require.NotNil(t, sourceVolume.Secret)
			require.Equal(t, "es-es-keystore-password", sourceVolume.Secret.SecretName)
			require.NotNil(t, sourceVolume.Secret.DefaultMode)
			require.Equal(t, int32(0440), *sourceVolume.Secret.DefaultMode)

			mainContainer := builder.MainContainer()
			require.NotNil(t, mainContainer)
			require.Contains(t, mainContainer.VolumeMounts, corev1.VolumeMount{
				Name:      VolumeName,
				MountPath: MountPath,
				ReadOnly:  true,
			})
			require.Contains(t, mainContainer.Env, corev1.EnvVar{
				Name:  "KEYSTORE_PASSWORD_FILE",
				Value: PasswordFile,
			})

			var keystoreInitContainer *corev1.Container
			for i := range builder.PodTemplate.Spec.InitContainers {
				if builder.PodTemplate.Spec.InitContainers[i].Name == keystore.InitContainerName {
					keystoreInitContainer = &builder.PodTemplate.Spec.InitContainers[i]
					break
				}
			}
			require.NotNil(t, keystoreInitContainer)
			require.Contains(t, keystoreInitContainer.VolumeMounts, corev1.VolumeMount{
				Name:      VolumeName,
				MountPath: MountPath,
				ReadOnly:  true,
			})
		})
	}
}

func TestApplyPasswordProtectedKeystoreScript(t *testing.T) {
	tests := []struct {
		name                string
		parameters          keystore.InitContainerParameters
		wantCustomScriptSet bool
	}{
		{
			name: "sets custom script when password path is configured",
			parameters: keystore.InitContainerParameters{
				KeystoreCreateCommand:         "/usr/share/elasticsearch/bin/elasticsearch-keystore create",
				KeystoreAddCommand:            `/usr/share/elasticsearch/bin/elasticsearch-keystore add-file "$key" "$filename"`,
				SecureSettingsVolumeMountPath: "/mnt/elastic-internal/secure-settings",
				KeystoreVolumePath:            "/usr/share/elasticsearch/config",
				KeystorePasswordPath:          PasswordFile,
			},
			wantCustomScriptSet: true,
		},
		{
			name: "does not set custom script without password path",
			parameters: keystore.InitContainerParameters{
				KeystoreCreateCommand:         "/usr/share/elasticsearch/bin/elasticsearch-keystore create",
				KeystoreAddCommand:            `/usr/share/elasticsearch/bin/elasticsearch-keystore add-file "$key" "$filename"`,
				SecureSettingsVolumeMountPath: "/mnt/elastic-internal/secure-settings",
				KeystoreVolumePath:            "/usr/share/elasticsearch/config",
			},
			wantCustomScriptSet: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyPasswordProtectedKeystoreScript(&tt.parameters)
			if !tt.wantCustomScriptSet {
				require.Empty(t, tt.parameters.CustomScript)
				return
			}

			require.NotEmpty(t, tt.parameters.CustomScript)
			rendered := renderCustomScript(t, tt.parameters)
			require.Contains(t, rendered, "rm -f /usr/share/elasticsearch/config/elasticsearch.keystore")
			require.Contains(t, rendered, "set +x")
			require.Contains(t, rendered, `KEYSTORE_PASSWORD=$(cat "/mnt/elastic-internal/keystore-password/keystore-password")`)
			require.Contains(t, rendered, `printf "%s\n%s\n" "$KEYSTORE_PASSWORD" "$KEYSTORE_PASSWORD" | /usr/share/elasticsearch/bin/elasticsearch-keystore create -p`)
			require.Contains(t, rendered, `echo -n "$KEYSTORE_PASSWORD" | /usr/share/elasticsearch/bin/elasticsearch-keystore add-file "$key" "$filename"`)
			require.Contains(t, rendered, "unset KEYSTORE_PASSWORD")
			require.Contains(t, rendered, "set -x")
		})
	}
}
