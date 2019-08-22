// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"testing"

	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileAssociation_reconcileCASecret(t *testing.T) {
	// setup scheme and init watches
	require.NoError(t, estype.AddToScheme(scheme.Scheme))
	require.NoError(t, kbtype.AddToScheme(scheme.Scheme))
	w := watches.NewDynamicWatches()
	require.NoError(t, w.Secrets.InjectScheme(scheme.Scheme))

	kibana := "kibana-foo"
	// mock existing ES resource
	es := estype.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esFixture.Namespace,
			Name:      esFixture.Name,
		},
	}
	// mock existing CA secret for ES
	esCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      certificates.PublicSecretName(esname.ESNamer, es.Name, certificates.HTTPCAType),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-ca-cert"),
		},
	}
	updatedEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      certificates.PublicSecretName(esname.ESNamer, es.Name, certificates.HTTPCAType),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("updated-fake-ca-cert"),
		},
	}
	// mock existing ES CA secret for Kibana
	kibanaEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      CACertSecretName(kibana),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-ca-cert"),
		},
	}
	updatedKibanaEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      CACertSecretName(kibana),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("updated-fake-ca-cert"),
		},
	}
	tests := []struct {
		name   string
		client k8s.Client
		kibana kbtype.Kibana
		es     estype.Elasticsearch
		want   string
		wantCA *corev1.Secret
	}{
		{
			name:   "create new CA in kibana namespace",
			client: k8s.WrapClient(fake.NewFakeClient(&es, &esCA)),
			kibana: kibanaFixture,
			es:     esFixture,
			want:   CACertSecretName(kibanaFixture.Name),
			wantCA: &kibanaEsCA,
		},
		{
			name:   "update existing CA in kibana namespace",
			client: k8s.WrapClient(fake.NewFakeClient(&es, &updatedEsCA, &kibanaEsCA)),
			kibana: kibanaFixture,
			es:     esFixture,
			want:   CACertSecretName(kibanaFixture.Name),
			wantCA: &updatedKibanaEsCA,
		},
		{
			name:   "ES CA secret does not exist (yet)",
			client: k8s.WrapClient(fake.NewFakeClient(&es)),
			kibana: kibanaFixture,
			es:     esFixture,
			want:   "",
			wantCA: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			r := &ReconcileAssociation{
				Client:  tt.client,
				scheme:  scheme.Scheme,
				watches: w,
			}

			got, err := r.reconcileCASecret(tt.kibana, k8s.ExtractNamespacedName(&tt.es))
			require.NoError(t, err)

			require.Equal(t, tt.want, got)

			if tt.wantCA != nil {
				var updatedKibanaCA corev1.Secret
				err = tt.client.Get(types.NamespacedName{
					Namespace: tt.kibana.Namespace,
					Name:      CACertSecretName(tt.kibana.Name),
				}, &updatedKibanaCA)
				require.NoError(t, err)
				require.Equal(t, tt.wantCA.Data, updatedKibanaCA.Data)
			}
		})
	}
}
