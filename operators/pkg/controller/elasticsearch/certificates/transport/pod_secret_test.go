// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package transport

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureTransportCertificateSecretExists(t *testing.T) {
	es := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-es",
		},
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod",
		},
	}

	defaultPodSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-certs",
			Labels: map[string]string{
				LabelCertificateType:       LabelCertificateTypeTransport,
				label.PodNameLabelName:     pod.Name,
				label.ClusterNameLabelName: es.Name,
			},
		},
	}

	defaultPodSecretWith := func(setter func(secret *corev1.Secret)) *corev1.Secret {
		secret := defaultPodSecret.DeepCopy()
		setter(secret)
		return secret
	}

	type args struct {
		c      k8s.Client
		scheme *runtime.Scheme
		owner  v1alpha1.Elasticsearch
		pod    corev1.Pod
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
				c:     k8s.WrapClient(fake.NewFakeClient()),
				owner: es,
				pod:   pod,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				// owner references are set upon creation, so ignore for comparison
				expected := defaultPodSecretWith(func(s *corev1.Secret) {
					s.OwnerReferences = secret.OwnerReferences
				})
				assert.Equal(t, expected, secret)
			},
		},
		{
			name: "should update an existing secret",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(defaultPodSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				}))),
				owner: es,
				pod:   pod,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				// UID should be kept the same
				assert.Equal(t, defaultPodSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.UID = types.UID("42")
				}), secret)
			},
		},
		{
			name: "should allow additional labels in the secret",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(defaultPodSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				}))),
				owner: es,
				pod:   pod,
			},
			want: func(t *testing.T, secret *corev1.Secret) {
				assert.Equal(t, defaultPodSecretWith(func(secret *corev1.Secret) {
					secret.ObjectMeta.Labels["foo"] = "bar"
				}), secret)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.args.scheme == nil {
				tt.args.scheme = scheme.Scheme
			}

			got, err := EnsureTransportCertificateSecretExists(tt.args.c, tt.args.scheme, tt.args.owner, tt.args.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureTransportCertificateSecretExists() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			tt.want(t, got)
		})
	}
}
