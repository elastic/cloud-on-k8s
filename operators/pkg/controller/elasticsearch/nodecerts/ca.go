// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package nodecerts

import (
	"bytes"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CASecretNameForCluster returns the name of the CA secret for the given cluster
func CASecretNameForCluster(clusterName string) string {
	return clusterName + "-ca"
}

// ReconcileCASecretForCluster ensures that a secret containing
// the CA certificate for the given cluster exists
func ReconcileCASecretForCluster(
	cl k8s.Client,
	ca *certificates.Ca,
	cluster v1alpha1.Elasticsearch,
	scheme *runtime.Scheme,
) error {
	expectedCABytes := certificates.EncodePEMCert(ca.Cert.Raw)
	clusterCASecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      CASecretNameForCluster(cluster.Name),
		},
		Data: map[string][]byte{
			certificates.CAFileName: expectedCABytes,
		},
	}

	reconciled := &corev1.Secret{}
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     cl,
		Scheme:     scheme,
		Owner:      &cluster,
		Expected:   &clusterCASecret,
		Reconciled: reconciled,
		NeedsUpdate: func() bool {
			if reconciled.Data == nil {
				return true
			}
			actualCABytes, exists := reconciled.Data[certificates.CAFileName]
			return !exists || !bytes.Equal(actualCABytes, expectedCABytes)

		},
		UpdateReconciled: func() {
			if reconciled.Data == nil {
				reconciled.Data = make(map[string][]byte)
			}
			reconciled.Data[certificates.CAFileName] = expectedCABytes
		},
	})
}
