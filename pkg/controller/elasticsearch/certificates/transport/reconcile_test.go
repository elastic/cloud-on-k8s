// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_ensureTransportCertificateSecretExists(t *testing.T) {
	defaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.TransportCertificatesSecret(testES.Name),
			Namespace: testES.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: testES.Name,
			},
		},
		Data: make(map[string][]byte),
	}

	defaultSecretWith := func(setter func(secret *corev1.Secret)) *corev1.Secret {
		secret := defaultSecret.DeepCopy()
		setter(secret)
		return secret
	}

	type args struct {
		c     k8s.Client
		owner esv1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    func(*testing.T, *corev1.Secret)
		wantErr bool
	}{
		{
			name: "should create a secret if it does not already exist",
			args: args{
				c:     k8s.WrappedFakeClient(),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				// owner references are set upon creation, so ignore for comparison
				expected := defaultSecretWith(func(s *corev1.Secret) {
					s.OwnerReferences = secret.OwnerReferences
				})
				comparison.AssertEqual(t, expected, secret)
			},
		},
		{
			name: "should update an existing secret",
			args: args{
				c: k8s.WrappedFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				// UID should be kept the same
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				}), secret)
			},
		},
		{
			name: "should not modify the secret data if already exists",
			args: args{
				c: k8s.WrappedFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
					secret.Data = map[string][]byte{
						"existing": []byte("data"),
					}
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				// UID and data should be kept
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
					secret.Data = map[string][]byte{
						"existing": []byte("data"),
					}
				}), secret)
			},
		},
		{
			name: "should allow additional labels in the secret",
			args: args{
				c: k8s.WrappedFakeClient(defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				})),
				owner: testES,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				comparison.AssertEqual(t, defaultSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				}), secret)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureTransportCertificatesSecretExists(tt.args.c, tt.args.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureTransportCertificateSecretExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}
