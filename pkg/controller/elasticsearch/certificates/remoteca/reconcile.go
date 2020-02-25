// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"bytes"
	"reflect"
	"sort"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/remotecluster/remoteca"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// SecretNameSuffix is a suffix for the secret that contains the concatenation of all the remote CAs
	SecretNameSuffix string = "remote-ca"
)

func SecretName(esName string) string {
	return esv1.ESNamer.Suffix(esName, SecretNameSuffix)
}

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
	i := 0
	for _, remoteCA := range remoteCAList.Items {
		remoteCertificateAuthorities[i] = remoteCA.Data[certificates.CAFileName]
		i++
	}

	expected := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SecretName(es.Name),
			Namespace: es.Namespace,
			Labels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		},
		Data: map[string][]byte{
			certificates.CAFileName: bytes.Join(remoteCertificateAuthorities, nil),
		},
	}

	var reconciled v1.Secret
	return reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme.Scheme,
		Owner:      &es,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !maps.IsSubset(expected.Labels, reconciled.Labels) || !reflect.DeepEqual(expected.Data, reconciled.Data)
		},
		UpdateReconciled: func() {
			reconciled.Labels = maps.Merge(reconciled.Labels, expected.Labels)
			reconciled.Data = expected.Data
		},
	})
}
