// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package shared

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func testElasticsearch(secureSettings ...commonv1.SecretSource) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "test-es", Namespace: "ns"},
		Spec:       esv1.ElasticsearchSpec{SecureSettings: secureSettings},
	}
}

func TestBuildClusterSecrets(t *testing.T) {
	tests := []struct {
		name              string
		secureSettings    []commonv1.SecretSource
		objects           []client.Object
		wantStringSecrets map[string]any
		wantWatchesEmpty  bool
	}{
		{
			name:              "no sources produces empty string_secrets",
			wantStringSecrets: map[string]any{},
			wantWatchesEmpty:  true,
		},
		{
			name:           "user secure settings are aggregated into string_secrets",
			secureSettings: []commonv1.SecretSource{{SecretName: "my-secure-settings"}},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-secure-settings"},
					Data: map[string][]byte{
						"s3.client.default.access_key": []byte("AKIAIOSFODNN7EXAMPLE"),
						"s3.client.default.secret_key": []byte("wJalrXUtnFEMI/K7MDENG"),
					},
				},
			},
			wantStringSecrets: map[string]any{
				"s3.client.default.access_key": "AKIAIOSFODNN7EXAMPLE",
				"s3.client.default.secret_key": "wJalrXUtnFEMI/K7MDENG",
			},
		},
		{
			name:              "missing source secret is silently skipped",
			secureSettings:    []commonv1.SecretSource{{SecretName: "missing-secret"}},
			wantStringSecrets: map[string]any{},
		},
		{
			name:           "watch is registered when a source secret is referenced",
			secureSettings: []commonv1.SecretSource{{SecretName: "watched-secret"}},
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "watched-secret"},
					Data:       map[string][]byte{"key": []byte("value")},
				},
			},
			wantStringSecrets: map[string]any{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := testElasticsearch(tt.secureSettings...)
			objects := make([]client.Object, 1, 1+len(tt.objects))
			objects[0] = &es
			objects = append(objects, tt.objects...)
			c := k8s.NewFakeClient(objects...)
			dw := watches.NewDynamicWatches()
			recorder := &toolsevents.FakeRecorder{}

			result, err := BuildClusterSecrets(context.Background(), c, recorder, dw, es, "")
			require.NoError(t, err)
			require.NotNil(t, result)

			stringSecrets, ok := result.Data["string_secrets"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantStringSecrets, stringSecrets)

			if tt.wantWatchesEmpty {
				assert.Empty(t, dw.Secrets.Registrations())
			} else {
				assert.NotEmpty(t, dw.Secrets.Registrations())
			}
		})
	}
}
