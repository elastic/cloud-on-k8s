// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace = "ns1"
	testName      = "mycluster"
)

func TestCASecretNameForCluster(t *testing.T) {
	require.Equal(t, "mycluster-ca", CASecretNameForCluster(testName))
}

func TestReconcileCASecretForCluster(t *testing.T) {
	// register es cluster type
	v1alpha1.AddToScheme(scheme.Scheme)

	ca, _ := certificates.NewSelfSignedCa("foo")
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testName,
		},
	}

	// Create an outdated secret
	c := k8s.WrapClient(fake.NewFakeClient(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      CASecretNameForCluster(testName),
			},
			Data: map[string][]byte{certificates.CAFileName: []byte("awronginitialsupersecret1")},
		}))

	// Reconciliation must update it
	err := ReconcileCASecretForCluster(c, ca, cluster, scheme.Scheme)
	require.NoError(t, err)

	// Check if the secret has been updated
	updated := &corev1.Secret{}
	c.Get(types.NamespacedName{
		Namespace: testNamespace,
		Name:      "mycluster-ca",
	}, updated)

	expectedCaKeyBytes := certificates.EncodePEMCert(ca.Cert.Raw)
	require.EqualValues(t, expectedCaKeyBytes, updated.Data[certificates.CAFileName])
}
