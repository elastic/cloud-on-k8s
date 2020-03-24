// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

var (
	rotation = RotationParams{
		Validity:     DefaultCertValidity,
		RotateBefore: DefaultRotateBefore,
	}
	labels = map[string]string{
		"foo": "bar",
	}
	// tested on Elasticsearch but could be any resource
	obj = esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
)

// this test just visits the main path of the certs reconciliation
// inner functions are individually tested elsewhere
func TestReconcileCAAndHTTPCerts(t *testing.T) {
	c := k8s.WrappedFakeClient()

	r := Reconciler{
		K8sClient:             c,
		DynamicWatches:        watches.NewDynamicWatches(),
		Object:                &obj,
		TLSOptions:            commonv1.TLSOptions{},
		Namer:                 esv1.ESNamer,
		Labels:                labels,
		Services:              nil,
		CACertRotation:        rotation,
		CertRotation:          rotation,
		GarbageCollectSecrets: false,
	}
	httpCerts, results := r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults := func() {
		aggregateResult, err := results.Aggregate()
		require.NoError(t, err)
		// a reconciliation should be requested later to deal with cert expiration
		require.NotZero(t, aggregateResult.RequeueAfter)
		// returned http certs should hold cert data
		require.NotNil(t, httpCerts)
		require.NotEmpty(t, httpCerts.Data)
	}
	checkResults()

	// the 3 secrets should have been created in the apiserver,
	// and have the expected labels and content generated
	checkCertsSecrets := func() {
		var caCerts corev1.Secret
		err := c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: CAInternalSecretName(esv1.ESNamer, obj.Name, HTTPCAType)}, &caCerts)
		require.NoError(t, err)
		require.Len(t, caCerts.Data, 2)
		require.NotEmpty(t, caCerts.Data[CertFileName])
		require.NotEmpty(t, caCerts.Data[KeyFileName])
		require.Equal(t, labels, caCerts.Labels)

		var internalCerts corev1.Secret
		err = c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: InternalCertsSecretName(esv1.ESNamer, obj.Name)}, &internalCerts)
		require.NoError(t, err)
		require.Len(t, internalCerts.Data, 3)
		require.NotEmpty(t, internalCerts.Data[CAFileName])
		require.NotEmpty(t, internalCerts.Data[CertFileName])
		require.NotEmpty(t, internalCerts.Data[KeyFileName])
		require.Equal(t, labels, internalCerts.Labels)

		var publicCerts corev1.Secret
		err = c.Get(types.NamespacedName{Namespace: obj.Namespace, Name: PublicCertsSecretName(esv1.ESNamer, obj.Name)}, &publicCerts)
		require.NoError(t, err)
		require.Len(t, publicCerts.Data, 2)
		require.NotEmpty(t, publicCerts.Data[CAFileName])
		require.NotEmpty(t, publicCerts.Data[CertFileName])
		require.Equal(t, labels, publicCerts.Labels)

	}
	checkCertsSecrets()

	// running again should lead to the same results
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults()
	checkCertsSecrets()

	// disable TLS and run again: should keep existing certs secrets (Elasticsearch use case)
	r.TLSOptions = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}
	r.GarbageCollectSecrets = false
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	checkResults()
	checkCertsSecrets()

	// disable TLS and run again, this time with the option to remove secrets (Kibana, APMServer, Enterprise Search use cases)
	r.TLSOptions = commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}
	r.GarbageCollectSecrets = true
	httpCerts, results = r.ReconcileCAAndHTTPCerts(context.Background())
	aggregateResult, err := results.Aggregate()
	require.NoError(t, err)
	require.Zero(t, aggregateResult.RequeueAfter)
	require.Nil(t, httpCerts)
	removedSecrets := []types.NamespacedName{
		{Namespace: obj.Namespace, Name: CAInternalSecretName(esv1.ESNamer, obj.Name, HTTPCAType)},
		{Namespace: obj.Namespace, Name: InternalCertsSecretName(esv1.ESNamer, obj.Name)},
		{Namespace: obj.Namespace, Name: PublicCertsSecretName(esv1.ESNamer, obj.Name)},
	}
	for _, nsn := range removedSecrets {
		var s corev1.Secret
		require.True(t, apierrors.IsNotFound(c.Get(nsn, &s)))
	}
}
