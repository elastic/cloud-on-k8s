// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_GetSecureSettingsSecretSourcesForResources(t *testing.T) {
	type args struct {
		resource     metav1.Object
		resourceKind string
		client       k8s.Client
	}

	kibanaConfigSecretFixture := MkKibanaConfigSecret("test-kb-ns", "test-policy", "test-policy-ns", "")
	addSecureSettingsAnnotationToSecret(kibanaConfigSecretFixture, "test-policy-ns")

	elasticsearchConfigSecretFixture := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-es-es-file-settings",
			Namespace: "test-es-ns",
		},
	}
	addSecureSettingsAnnotationToSecret(elasticsearchConfigSecretFixture, "test-policy-ns")

	tests := []struct {
		name string
		args args
		want []commonv1.NamespacedSecretSource
	}{
		{
			name: "Get secure settings secrets for Kibana",
			args: args{
				resource: &kibanav1.Kibana{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-kb",
						Namespace: "test-kb-ns",
					},
				},
				resourceKind: "Kibana",
				client:       k8s.NewFakeClient(kibanaConfigSecretFixture),
			},
			want: []commonv1.NamespacedSecretSource{
				{
					SecretName: "shared-secret",
					Namespace:  "test-policy-ns",
				},
			},
		},
		{
			name: "Get secure settings secrets for Elasticsearch",
			args: args{
				resource: &esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es",
						Namespace: "test-es-ns",
					},
				},
				resourceKind: "Elasticsearch",
				client:       k8s.NewFakeClient(elasticsearchConfigSecretFixture),
			},
			want: []commonv1.NamespacedSecretSource{
				{
					SecretName: "shared-secret",
					Namespace:  "test-policy-ns",
				},
			},
		},
		{
			name: "secure settings for unknow kind",
			args: args{
				resourceKind: "UnknownKind",
				client:       k8s.NewFakeClient(elasticsearchConfigSecretFixture),
			},
			want: []commonv1.NamespacedSecretSource{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSecureSettingsSecretSourcesForResources(context.Background(), tt.args.client, tt.args.resource, tt.args.resourceKind)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func addSecureSettingsAnnotationToSecret(secret *corev1.Secret, namespace string) {
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["policy.k8s.elastic.co/secure-settings-secrets"] = fmt.Sprintf(`[{"namespace":"%s","secretName":"shared-secret"}]`, namespace)
}
