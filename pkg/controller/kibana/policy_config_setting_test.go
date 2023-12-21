// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_getPolicyConfig(t *testing.T) {
	canonicalConfig := common.MustCanonicalConfig(map[string]interface{}{
		"xpack.canvas.enabled": true,
	})
	for _, tt := range []struct {
		name    string
		kb      kibanav1.Kibana
		want    PolicyConfig
		wantErr bool
		client  k8s.Client
	}{
		{
			name: "create valid policy config",
			kb: kibanav1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test-ns",
				},
			},
			want: PolicyConfig{
				KibanaConfig: canonicalConfig,
				PodAnnotations: map[string]string{
					"policy.k8s.elastic.co/kibana-config-hash": "123456",
				},
			},
			client: k8s.NewFakeClient(mkKibanaConfigSecret("test-ns", "test-policy", "test-policy-ns", "123456")),
		},
		{
			name: "create invalid policy config",
			kb: kibanav1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test-ns",
				},
			},
			want: PolicyConfig{
				PodAnnotations: map[string]string{
					"policy.k8s.elastic.co/kibana-config-hash": "123456",
				},
			},
			wantErr: true,
			client:  k8s.NewFakeClient(mkInvalidConfigSecret("test-ns", "test-policy", "test-policy-ns", "123456")),
		},
		{
			name: "policy kibana config secret not present",
			kb: kibanav1.Kibana{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-kb",
					Namespace: "test-ns",
				},
			},
			want:    PolicyConfig{},
			wantErr: false,
			client:  k8s.NewFakeClient(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPolicyConfig(context.Background(), tt.client, tt.kb)
			if !tt.wantErr {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func mkKibanaConfigSecret(namespace string, owningPolicyName string, owningPolicyNamespace string, hashValue string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test-kb-kb-policy-config",
			Labels: map[string]string{
				"asset.policy.k8s.elastic.co/on-delete": "delete",
				"kibana.k8s.elastic.co/name":            "test-kb",
				"common.k8s.elastic.co/type":            "kibana",
				"eck.k8s.elastic.co/owner-kind":         "StackConfigPolicy",
				"eck.k8s.elastic.co/owner-name":         owningPolicyName,
				"eck.k8s.elastic.co/owner-namespace":    owningPolicyNamespace,
			},
			Annotations: map[string]string{
				"policy.k8s.elastic.co/kibana-config-hash": hashValue,
			},
		},
		Data: map[string][]byte{
			"kibana.json": []byte(`{"xpack.canvas.enabled":true}`),
		},
	}
}

func mkInvalidConfigSecret(namespace string, owningPolicyName string, owningPolicyNamespace string, hashValue string) *corev1.Secret {
	secret := mkKibanaConfigSecret(namespace, owningPolicyName, owningPolicyNamespace, hashValue)
	secret.Data = map[string][]byte{
		"kibana.json": []byte(`test`),
	}
	return secret
}
