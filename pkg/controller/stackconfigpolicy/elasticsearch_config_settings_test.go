// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_reconcileSecretMountSecretsESNamespace(t *testing.T) {
	type args struct {
		client k8s.Client
		es     esv1.Elasticsearch
		policy *policyv1alpha1.StackConfigPolicy
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Create secret mount secrets in ES namespace",
			args: args{
				client: k8s.NewFakeClient(getSecretMountSecret(t, "auth-policy-secret", "test-policy-ns", "test-policy", "test-policy-ns", "delete")),
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							SecretMounts: []policyv1alpha1.SecretMount{
								{
									SecretName: "auth-policy-secret",
									MountPath:  "/usr/test",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Secret mount secret in policy does not exist",
			args: args{
				client: k8s.NewFakeClient(),
				es: esv1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-es",
						Namespace: "test-ns",
					},
				},
				policy: &policyv1alpha1.StackConfigPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-policy",
						Namespace: "test-policy-ns",
					},
					Spec: policyv1alpha1.StackConfigPolicySpec{
						Elasticsearch: policyv1alpha1.ElasticsearchConfigPolicySpec{
							SecretMounts: []policyv1alpha1.SecretMount{
								{
									SecretName: "auth-policy-secret",
									MountPath:  "/usr/test",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := reconcileSecretMounts(context.TODO(), tt.args.client, tt.args.es, tt.args.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify secret was created in es namespace
			if err == nil {
				for _, secretMount := range tt.args.policy.Spec.Elasticsearch.SecretMounts {
					expectedSecret := &corev1.Secret{}
					expectedNsn := types.NamespacedName{
						Name:      esv1.StackConfigAdditionalSecretName(tt.args.es.Name, secretMount.SecretName),
						Namespace: "test-ns",
					}
					err := tt.args.client.Get(context.TODO(), expectedNsn, expectedSecret)
					if (err != nil) != tt.wantErr {
						t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
						return
					}

					require.Equal(t, expectedSecret.Data, getSecretMountSecret(t, esv1.ESNamer.Suffix(tt.args.es.Name, secretMount.SecretName), "test-ns", "test-policy", "test-policy-ns", "delete").Data, "secrets do not match")
				}
			}
		})
	}
}

func getSecretMountSecret(t *testing.T, name string, namespace string, policyName string, policyNamespace string, orphanObjectOnPolicyDeleteStratergy string) *corev1.Secret {
	t.Helper()
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"elasticsearch.k8s.elastic.co/cluster-name": "another-es",
				"common.k8s.elastic.co/type":                "elasticsearch",
				"asset.policy.k8s.elastic.co/on-delete":     orphanObjectOnPolicyDeleteStratergy,
				"eck.k8s.elastic.co/owner-namespace":        policyNamespace,
				"eck.k8s.elastic.co/owner-name":             policyName,
				"eck.k8s.elastic.co/owner-kind":             policyv1alpha1.Kind,
			},
		},
		Data: map[string][]byte{
			"idfile.txt": []byte("test id file"),
		},
	}
}
