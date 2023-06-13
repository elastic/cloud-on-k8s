// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remoteca

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestReconcile(t *testing.T) {
	type args struct {
		es          esv1.Elasticsearch
		secrets     []client.Object
		transportCA certificates.CA
	}
	testTransportCA, _ := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "Certificates should be sorted",
			args: args{
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "ns1"}},
				secrets: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "b",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								commonv1.TypeLabelName:     TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert1\n")},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "a",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								commonv1.TypeLabelName:     TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert2\n")},
					},
				},
				transportCA: *testTransportCA,
			},
			want: []byte("cert2\ncert1\n"),
		},
		{
			name: "Only include Secrets with the right label",
			args: args{
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "ns1"}},
				secrets: []client.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "b",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								commonv1.TypeLabelName:     TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert1\n")},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "c",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								commonv1.TypeLabelName:     "foo",
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert3\n")},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "a",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								commonv1.TypeLabelName:     TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert2\n")},
					},
				},
				transportCA: *testTransportCA,
			},
			want: []byte("cert2\ncert1\n"),
		},
		{
			name: "Use provided transport CA if remote CA list is empty",
			args: args{
				es:          esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "ns1"}},
				transportCA: *testTransportCA,
			},
			want: certificates.EncodePEMCert(testTransportCA.Cert.Raw),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.NewFakeClient(tt.args.secrets...)
			if err := Reconcile(context.Background(), k8sClient, tt.args.es, tt.args.transportCA); (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			remoteCaList := v1.Secret{}
			assert.NoError(t, k8sClient.Get(context.Background(), types.NamespacedName{Namespace: "ns1", Name: "es1-es-remote-ca"}, &remoteCaList))
			content, ok := remoteCaList.Data[certificates.CAFileName]
			assert.True(t, ok)
			assert.Equal(t, tt.want, content)
		})
	}
}
