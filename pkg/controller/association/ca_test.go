// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const kibanaESAssociationName = "kibana-es"

func TestReconcileAssociation_reconcileCASecret(t *testing.T) {
	esFixture := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "es-foo",
			Namespace: "default",
		},
	}
	kibanaFixtureObjectMeta := metav1.ObjectMeta{
		Name:      "kibana-foo",
		Namespace: "default",
	}
	kibanaFixture := kbv1.Kibana{
		ObjectMeta: kibanaFixtureObjectMeta,
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{
				Name:      esFixture.Name,
				Namespace: esFixture.Namespace,
			},
		},
	}
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
			Name:      CACertSecretName(&kibanaFixture, kibanaESAssociationName),
		},
		Data: map[string][]byte{
			certificates.CertFileName: []byte("fake-cert"),
			certificates.CAFileName:   []byte("fake-ca-cert"),
		},
	}
	updatedKibanaEsCA := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: es.Namespace,
			Name:      CACertSecretName(&kibanaFixture, kibanaESAssociationName),
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
			Name:      CACertSecretName(&kibanaFixture, kibanaESAssociationName),
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
			client:             k8s.NewFakeClient(&es, &esCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               CACertSecretName(&kibanaFixture, kibanaESAssociationName),
			wantCA:             &kibanaEsCA,
			wantCACertProvided: true,
		},
		{
			name:               "update existing CA in kibana namespace",
			client:             k8s.NewFakeClient(&es, &updatedEsCA, &kibanaEsCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               CACertSecretName(&kibanaFixture, kibanaESAssociationName),
			wantCA:             &updatedKibanaEsCA,
			wantCACertProvided: true,
		},
		{
			name:               "ES CA secret does not exist (yet)",
			client:             k8s.NewFakeClient(&es),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               "",
			wantCA:             nil,
			wantCACertProvided: false,
		},
		{
			// See the use case described in https://github.com/elastic/cloud-on-k8s/issues/2136
			name:               "ES CA secret exists but is empty",
			client:             k8s.NewFakeClient(&es, &esEmptyCA),
			kibana:             kibanaFixture,
			es:                 esFixture,
			want:               CACertSecretName(&kibanaFixture, kibanaESAssociationName),
			wantCA:             &kibanaEmptyEsCA,
			wantCACertProvided: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				AssociationInfo: AssociationInfo{
					Labels: func(associated types.NamespacedName) map[string]string {
						return map[string]string{}
					},
					AssociationName:                       "kibana-es",
					AssociationResourceNameLabelName:      "elasticsearch.k8s.elastic.co/cluster-name",
					AssociationResourceNamespaceLabelName: "elasticsearch.k8s.elastic.co/cluster-namespace",
				},
				Client:     tt.client,
				watches:    watches.DynamicWatches{},
				Parameters: operator.Parameters{},
			}

			// re-use the one used for ES association, but it could be anything else
			caSecretServiceLabelName := "elasticsearch.k8s.elastic.co/cluster-name"

			got, err := r.ReconcileCASecret(
				&tt.kibana,
				esv1.ESNamer,
				k8s.ExtractNamespacedName(&tt.es),
			)
			require.NoError(t, err)

			require.Equal(t, tt.want, got.Name)
			require.Equal(t, tt.wantCACertProvided, got.CACertProvided)

			if tt.wantCA != nil {
				var updatedKibanaCA corev1.Secret
				err = tt.client.Get(context.Background(), types.NamespacedName{
					Namespace: tt.kibana.Namespace,
					Name:      CACertSecretName(&kibanaFixture, "kibana-es"),
				}, &updatedKibanaCA)
				require.NoError(t, err)
				require.Equal(t, tt.wantCA.Data, updatedKibanaCA.Data)
				require.True(t, len(updatedKibanaCA.Labels) > 0)
				serviceLabelValue, ok := updatedKibanaCA.Labels[caSecretServiceLabelName]
				require.True(t, ok)
				require.Equal(t, tt.es.Name, serviceLabelValue)
			}
		})
	}
}
