// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/ca"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/certutils"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// this test just visits the main path of the certs reconciliation
// inner functions are individually tested elsewhere
func TestReconcileCAAndHTTPCerts(t *testing.T) {
	rotation := certutils.RotationParams{
		Validity:     certutils.DefaultCertValidity,
		RotateBefore: certutils.DefaultRotateBefore,
	}
	labels := map[string]string{
		"foo": "bar",
	}
	// tested on Elasticsearch but could be any resource
	obj := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	tests := []struct {
		name       string
		object     metav1.Object
		tlsOptions commonv1.TLSOptions
	}{
		{
			name:       "reconcile 3 secrets",
			object:     &obj,
			tlsOptions: commonv1.TLSOptions{},
		},
		{
			name:       "reconcile 3 secrets, even if TLS is disabled",
			object:     &obj,
			tlsOptions: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.WrappedFakeClient()
			httpCerts, results := ReconcileCAAndHTTPCerts(context.Background(), tt.object, tt.tlsOptions, labels, esv1.ESNamer, c, watches.NewDynamicWatches(), nil, rotation, rotation)
			aggregateResult, err := results.Aggregate()
			require.NoError(t, err)
			// a reconciliation should be requested later to deal with cert expiration
			require.NotZero(t, aggregateResult.RequeueAfter)
			// return http certs should hold cert data
			require.NotNil(t, httpCerts)
			require.NotEmpty(t, httpCerts.Data)

			// the 3 secrets should have been created in the apiserver,
			// and have the expected labels and content generated
			var caCerts corev1.Secret
			err = c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: ca.CAInternalSecretName(esv1.ESNamer, obj.Name, ca.HTTPCAType)}, &caCerts)
			require.NoError(t, err)
			require.Len(t, caCerts.Data, 2)
			require.NotEmpty(t, caCerts.Data[certutils.CertFileName])
			require.NotEmpty(t, caCerts.Data[certutils.KeyFileName])
			require.Equal(t, labels, caCerts.Labels)

			var internalCerts corev1.Secret
			err = c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: http.InternalCertsSecretName(esv1.ESNamer, obj.Name)}, &internalCerts)
			require.NoError(t, err)
			require.Len(t, internalCerts.Data, 3)
			require.NotEmpty(t, internalCerts.Data[certutils.CAFileName])
			require.NotEmpty(t, internalCerts.Data[certutils.CertFileName])
			require.NotEmpty(t, internalCerts.Data[certutils.KeyFileName])
			require.Equal(t, labels, internalCerts.Labels)

			var publicCerts corev1.Secret
			err = c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: http.PublicCertsSecretName(esv1.ESNamer, obj.Name)}, &publicCerts)
			require.NoError(t, err)
			require.Len(t, publicCerts.Data, 2)
			require.NotEmpty(t, publicCerts.Data[certutils.CAFileName])
			require.NotEmpty(t, publicCerts.Data[certutils.CertFileName])
			require.Equal(t, labels, publicCerts.Labels)
		})
	}
}
