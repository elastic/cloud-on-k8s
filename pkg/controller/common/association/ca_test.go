// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

const ElasticsearchCASecretSuffix = "xx-es-ca" // nolint

func TestReconcileAssociation_reconcileCASecret(t *testing.T) {
	// mock existing ES resource
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: esFixture.Namespace,
			Name:      esFixture.Name,
		},
	}
	// mock existing CA secret for ES
	esCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-cert"),
			certificates.CAFileName:   []byte("fake-ca-cert"),
		},
	}
	updatedEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("updated-fake-cert"),
			certificates.CAFileName:   []byte("updated-fake-ca-cert"),
		},
	}
	// mock existing ES CA secret for Kibana
	kibanaEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-cert"),
			certificates.CAFileName:   []byte("fake-ca-cert"),
		},
	}
	updatedKibanaEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("updated-fake-cert"),
			certificates.CAFileName:   []byte("updated-fake-ca-cert"),
		},
	}
	esEmptyCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      certificates.PublicCertsSecretName(esv1.ESNamer, es.Name),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-cert"),
			certificates.CAFileName:   {},
		},
	}
	kibanaEmptyEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-cert"),
			certificates.CAFileName:   {},
		},
	}
	tests := []struct {
		name               string
		client             k8s.Client
		kibana             kbv1.Kibana
		es                 esv1.Elasticsearch
		want               string
		wantCA             *corev1.Secret
		wantCACertProvided bool
	}{
		{
			name:               "create new CA in kibana namespace",
			client:             k8s.WrappedFakeClient(&es, &esCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
			wantCA:             &kibanaEsCA,
			wantCACertProvided: true,
		},
		{
			name:               "update existing CA in kibana namespace",
			client:             k8s.WrappedFakeClient(&es, &updatedEsCA, &kibanaEsCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
			wantCA:             &updatedKibanaEsCA,
			wantCACertProvided: true,
		},
		{
			name:               "ES CA secret does not exist (yet)",
			client:             k8s.WrappedFakeClient(&es),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               "",
			wantCA:             nil,
			wantCACertProvided: false,
		},
		{
			// See the use case described in https://github.com/elastic/cloud-on-k8s/issues/2136
			name:               "ES CA secret exists but is empty",
			client:             k8s.WrappedFakeClient(&es, &esEmptyCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
			wantCA:             &kibanaEmptyEsCA,
			wantCACertProvided: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReconcileCASecret(
				tt.client,
				&tt.kibana,
				k8s.ExtractNamespacedName(&tt.es),
				map[string]string{},
				ElasticsearchCASecretSuffix,
			)
			require.NoError(t, err)

			require.Equal(t, tt.want, got.Name)
			require.Equal(t, tt.wantCACertProvided, got.CACertProvided)

			if tt.wantCA != nil {
				var updatedKibanaCA corev1.Secret
				err = tt.client.Get(types.NamespacedName{
					Namespace: tt.kibana.Namespace,
					Name:      ElasticsearchCACertSecretName(&kibanaFixture, ElasticsearchCASecretSuffix),
				}, &updatedKibanaCA)
				require.NoError(t, err)
				require.Equal(t, tt.wantCA.Data, updatedKibanaCA.Data)
			}
		})
	}
}
