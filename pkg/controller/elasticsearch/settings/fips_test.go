// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestAnyNodeSetFIPSEnabled(t *testing.T) {
	fipsConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": true,
	})
	nonFIPSConfig := commonv1.NewConfig(map[string]any{
		"xpack.security.fips_mode.enabled": false,
	})

	tests := []struct {
		name         string
		nodeSets     []esv1.NodeSet
		policyConfig *common.CanonicalConfig
		want         bool
	}{
		{
			name:     "fips enabled in nodeset config as boolean",
			nodeSets: []esv1.NodeSet{{Name: "data", Config: &fipsConfig}},
			want:     true,
		},
		{
			name: "fips enabled in nodeset config as string",
			nodeSets: []esv1.NodeSet{{
				Name: "data",
				Config: func() *commonv1.Config {
					c := commonv1.NewConfig(map[string]any{"xpack.security.fips_mode.enabled": "true"})
					return &c
				}(),
			}},
			want: true,
		},
		{
			name:     "fips disabled in nodeset config",
			nodeSets: []esv1.NodeSet{{Name: "data", Config: &nonFIPSConfig}},
			want:     false,
		},
		{
			name:     "fips setting missing from nodeset config",
			nodeSets: []esv1.NodeSet{{Name: "data"}},
			want:     false,
		},
		{
			name: "mixed nodesets, one fips enabled",
			nodeSets: []esv1.NodeSet{
				{Name: "master", Config: &nonFIPSConfig},
				{Name: "data", Config: &fipsConfig},
			},
			want: true,
		},
		{
			name:         "fips enabled via policy config only",
			nodeSets:     []esv1.NodeSet{{Name: "data"}},
			policyConfig: common.MustCanonicalConfig(map[string]any{"xpack.security.fips_mode.enabled": true}),
			want:         true,
		},
		{
			name:         "policy config without fips, nodeset without fips",
			nodeSets:     []esv1.NodeSet{{Name: "data", Config: &nonFIPSConfig}},
			policyConfig: common.MustCanonicalConfig(map[string]any{}),
			want:         false,
		},
		{
			name:         "nil policy config, no nodesets",
			nodeSets:     nil,
			policyConfig: nil,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnyNodeSetFIPSEnabled(tt.nodeSets, tt.policyConfig)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestHasUserProvidedKeystorePassword(t *testing.T) {
	const namespace = "ns"

	tests := []struct {
		name        string
		objects     []client.Object
		podTemplate corev1.PodTemplateSpec
		want        bool
		wantErr     bool
	}{
		{
			name: "contains KEYSTORE_PASSWORD",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD", Value: "ignored"}},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "contains ES_KEYSTORE_PASSPHRASE_FILE",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "ES_KEYSTORE_PASSPHRASE_FILE", Value: "/tmp/passphrase"}},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "missing both keystore env vars",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							Env:  []corev1.EnvVar{{Name: "SOME_OTHER_VAR", Value: "x"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "missing elasticsearch container",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "sidecar",
							Env:  []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD", Value: "ignored"}},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "envFrom ConfigMap containing KEYSTORE_PASSWORD",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "my-configmap"},
					Data:       map[string]string{"KEYSTORE_PASSWORD": "from-cm"},
				},
			},
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-configmap"},
								}},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "envFrom Secret containing ES_KEYSTORE_PASSPHRASE_FILE",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "my-secret"},
					Data:       map[string][]byte{"ES_KEYSTORE_PASSPHRASE_FILE": []byte("/tmp/pw")},
				},
			},
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
								}},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "envFrom ConfigMap without keystore vars",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "unrelated-cm"},
					Data:       map[string]string{"SOME_OTHER_VAR": "val"},
				},
			},
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "unrelated-cm"},
								}},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "envFrom prefix causes key to match",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "prefixed-secret"},
					Data:       map[string][]byte{"PASSWORD": []byte("val")},
				},
			},
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{
									Prefix: "KEYSTORE_",
									SecretRef: &corev1.SecretEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "prefixed-secret"},
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "envFrom prefix prevents match",
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "prefixed-cm"},
					Data:       map[string]string{"KEYSTORE_PASSWORD": "val"},
				},
			},
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{
									Prefix: "MY_",
									ConfigMapRef: &corev1.ConfigMapEnvSource{
										LocalObjectReference: corev1.LocalObjectReference{Name: "prefixed-cm"},
									},
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "envFrom references missing ConfigMap",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "does-not-exist"},
								}},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "optional missing envFrom ConfigMap does not error",
			podTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: esv1.ElasticsearchContainerName,
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "does-not-exist"},
									Optional:             boolPtr(true),
								}},
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			got, err := HasUserProvidedKeystorePassword(context.Background(), c, namespace, tt.podTemplate)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAnyNodeSetHasUserProvidedKeystorePassword(t *testing.T) {
	const namespace = "ns"
	tests := []struct {
		name     string
		objects  []client.Object
		nodeSets []esv1.NodeSet
		want     bool
		wantErr  bool
	}{
		{
			name: "at least one nodeset has override",
			nodeSets: []esv1.NodeSet{
				{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: esv1.ElasticsearchContainerName},
							},
						},
					},
				},
				{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: esv1.ElasticsearchContainerName, Env: []corev1.EnvVar{{Name: "KEYSTORE_PASSWORD", Value: "pw"}}},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "missing envFrom object in any nodeset returns error",
			nodeSets: []esv1.NodeSet{
				{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name: esv1.ElasticsearchContainerName,
									EnvFrom: []corev1.EnvFromSource{
										{ConfigMapRef: &corev1.ConfigMapEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{Name: "missing"},
										}},
									},
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no nodeset override",
			nodeSets: []esv1.NodeSet{
				{
					PodTemplate: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{Name: esv1.ElasticsearchContainerName, Env: []corev1.EnvVar{{Name: "SOME_VAR", Value: "x"}}},
							},
						},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			got, err := AnyNodeSetHasUserProvidedKeystorePassword(context.Background(), c, namespace, tt.nodeSets)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func boolPtr(v bool) *bool {
	return &v
}
