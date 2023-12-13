// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_getPolicyConfig(t *testing.T) {
	canonicalConfig := common.MustCanonicalConfig(map[string]interface{}{
		"logger.org.elasticsearch.discovery": "DEBUG",
	})
	for _, tt := range []struct {
		name         string
		es           esv1.Elasticsearch
		configSecret corev1.Secret
		want         PolicyConfig
		wantErr      bool
	}{
		{
			name: "create valid policy config",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "test-ns",
				},
			},
			configSecret: mkConfigSecret(esv1.StackConfigElasticsearchConfigSecretName("test-es"), "test-ns"),
			want: PolicyConfig{
				ElasticsearchConfig: canonicalConfig,
				PolicyAnnotations: map[string]string{
					"policy.k8s.elastic.co/elasticsearch-config-mounts-hash": "testhash",
				},
				AdditionalVolumes: []volume.VolumeLike{
					volume.NewSecretVolumeWithMountPath(esv1.StackConfigAdditionalSecretName("test-es", "test1"), esv1.StackConfigAdditionalSecretName("test-es", "test1"), "/usr/test"),
				},
			},
		},
		{
			name: "create policy config when secret does not exist",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "test-ns",
				},
			},
			want: PolicyConfig{
				ElasticsearchConfig: common.MustCanonicalConfig(map[string]interface{}{}),
				PolicyAnnotations: map[string]string{
					"policy.k8s.elastic.co/elasticsearch-config-mounts-hash": "",
				},
				AdditionalVolumes: nil,
			},
		},
		{
			name: "invalid config",
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-es",
					Namespace: "test-ns",
				},
			},
			configSecret: mkInvalidConfigSecret(esv1.StackConfigElasticsearchConfigSecretName("test-es"), "test-ns"),
			want: PolicyConfig{
				ElasticsearchConfig: nil,
				PolicyAnnotations: map[string]string{
					"policy.k8s.elastic.co/elasticsearch-config-mounts-hash": "testhash",
				},
				AdditionalVolumes: nil,
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewFakeClient(&tt.configSecret)
			got, err := getPolicyConfig(context.Background(), client, tt.es)
			if !tt.wantErr {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func mkConfigSecret(name string, namespace string) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: "testhash",
			},
		},
		Data: map[string][]byte{stackconfigpolicy.ElasticSearchConfigKey: []byte(`{"logger.org.elasticsearch.discovery": "DEBUG"}`),
			stackconfigpolicy.SecretsMountKey: []byte(`[{"secretName": "test1", "mountPath": "/usr/test"}]`)},
	}
}

func mkInvalidConfigSecret(name string, namespace string) corev1.Secret {
	secret := mkConfigSecret(name, namespace)
	secret.Data = map[string][]byte{stackconfigpolicy.ElasticSearchConfigKey: []byte(`{"invalid"}`),
		stackconfigpolicy.SecretsMountKey: []byte(`[{"secretName": "test1", "mountPath": "/usr/test"}]`)}
	return secret
}
