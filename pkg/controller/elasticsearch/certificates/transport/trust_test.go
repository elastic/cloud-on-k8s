// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package transport

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	v1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestMaybeRetrieveAdditionalCAs(t *testing.T) {
	equalError := func(msg string) assert.ErrorAssertionFunc {
		return func(t assert.TestingT, err error, msgAndArgs ...interface{}) bool {
			return assert.EqualError(t, err, msg, msgAndArgs)
		}
	}

	type args struct {
		client        k8s.Client
		elasticsearch v1.Elasticsearch
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Noop if no extra CA defined",
			args: args{
				client:        k8s.NewFakeClient(),
				elasticsearch: v1.Elasticsearch{},
			},
			want:    nil,
			wantErr: assert.NoError,
		},
		{
			name: "NOK specified config map does not exist",
			args: args{
				client: k8s.NewFakeClient(),
				elasticsearch: v1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					Spec:       v1.ElasticsearchSpec{Transport: v1.TransportConfig{TLS: v1.TransportTLSOptions{CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-trust"}}}},
				},
			},
			want:    nil,
			wantErr: equalError("could not retrieve config map ns/my-trust specified in spec.transport.tls.certificateAuthorities: configmaps \"my-trust\" not found"),
		},
		{

			name: "NOK ca.crt in configmap does not exist",
			args: args{
				client: k8s.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-trust"}}),
				elasticsearch: v1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					Spec:       v1.ElasticsearchSpec{Transport: v1.TransportConfig{TLS: v1.TransportTLSOptions{CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-trust"}}}},
				},
			},
			want:    nil,
			wantErr: equalError("config map ns/my-trust specified in spec.transport.tls.certificateAuthorities must contain ca.crt file"),
		},
		{
			name: "OK happy path",
			args: args{
				client: k8s.NewFakeClient(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "my-trust"}, Data: map[string]string{"ca.crt": "CA bytes go here"}}),
				elasticsearch: v1.Elasticsearch{
					ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es"},
					Spec:       v1.ElasticsearchSpec{Transport: v1.TransportConfig{TLS: v1.TransportTLSOptions{CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-trust"}}}},
				},
			},
			want:    []byte("CA bytes go here"),
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileAdditionalCAs(context.Background(), tt.args.client, tt.args.elasticsearch, watches.NewDynamicWatches())
			if !tt.wantErr(t, err, fmt.Sprintf("ReconcileAdditionalCAs(%v, %v)", tt.args.client, tt.args.elasticsearch)) {
				return
			}
			assert.Equalf(t, tt.want, got, "ReconcileAdditionalCAs(%v, %v)", tt.args.client, tt.args.elasticsearch)
		})
	}
}
