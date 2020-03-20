// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"bytes"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// Reconcile fetches the list of remote certificate authorities and concatenates them into a single Secret
func Reconcile(
	c k8s.Client,
	es esv1.Elasticsearch,
) error {
	// Get all the remote certificate authorities
	var remoteCAList v1.SecretList
	if err := c.List(
		&remoteCAList,
		client.InNamespace(es.Namespace),
		remoteca.LabelSelector(es.Name),
	); err != nil {
		return err
	}
	// We sort the remote certificate authorities to have a stable comparison with the reconciled data
	sort.SliceStable(remoteCAList.Items, func(i, j int) bool {
		// We don't need to compare the namespace because they are all in the same one
		return remoteCAList.Items[i].Name < remoteCAList.Items[j].Name
	})

	remoteCertificateAuthorities := make([][]byte, len(remoteCAList.Items))
	for i, remoteCA := range remoteCAList.Items {
		remoteCertificateAuthorities[i] = remoteCA.Data[certificates.CAFileName]
	}

	expected := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esv1.RemoteCaSecretName(es.Name),
			Namespace: es.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		},
		Data: map[string][]byte{
			certificates.CAFileName: bytes.Join(remoteCertificateAuthorities, nil),
		},
	}
	_, err := reconciler.ReconcileSecret(c, expected, &es)
	return err
}
