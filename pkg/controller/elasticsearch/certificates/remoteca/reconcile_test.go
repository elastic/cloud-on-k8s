// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

func TestReconcile(t *testing.T) {
	type args struct {
		es      esv1.Elasticsearch
		secrets []runtime.Object
	}
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
				secrets: []runtime.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "b",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								common.TypeLabelName:       remoteca.TypeLabelValue,
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
								common.TypeLabelName:       remoteca.TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert2\n")},
					},
				},
			},
			want: []byte("cert2\ncert1\n"),
		},
		{
			name: "Only include Secrets with the right label",
			args: args{
				es: esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "ns1"}},
				secrets: []runtime.Object{
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "b",
							Namespace: "ns1",
							Labels: map[string]string{
								label.ClusterNameLabelName: "es1",
								common.TypeLabelName:       remoteca.TypeLabelValue,
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
								common.TypeLabelName:       "foo",
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
								common.TypeLabelName:       remoteca.TypeLabelValue,
							},
						},
						Data: map[string][]byte{certificates.CAFileName: []byte("cert2\n")},
					},
				},
			},
			want: []byte("cert2\ncert1\n"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k8sClient := k8s.WrappedFakeClient(tt.args.secrets...)
			if err := Reconcile(k8sClient, tt.args.es); (err != nil) != tt.wantErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			remoteCaList := v1.Secret{}
			assert.NoError(t, k8sClient.Get(types.NamespacedName{Namespace: "ns1", Name: "es1-es-remote-ca"}, &remoteCaList))
			content, ok := remoteCaList.Data[certificates.CAFileName]
			assert.True(t, ok)
			assert.Equal(t, tt.want, content)
		})
	}
}
