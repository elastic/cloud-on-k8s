// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remoteca

import (
	"bytes"
	"context"
	"sort"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TypeLabelValue is a type used to identify a Secret which contains the CA of a remote cluster.
	TypeLabelValue = "remote-ca"
)

func Labels(esName string) client.MatchingLabels {
	return map[string]string{
		label.ClusterNameLabelName: esName,
		common.TypeLabelName:       TypeLabelValue,
	}
}

// Reconcile fetches the list of remote certificate authorities and concatenates them into a single Secret
func Reconcile(
	c k8s.Client,
	es esv1.Elasticsearch,
	transportCA certificates.CA,
) error {
	// Get all the remote certificate authorities
	var remoteCAList v1.SecretList
	if err := c.List(context.Background(),
		&remoteCAList,
		client.InNamespace(es.Namespace),
		Labels(es.Name),
	); err != nil {
		return err
	}

	var remoteCertificateAuthorities [][]byte
	if len(remoteCAList.Items) > 0 {
		// We sort the remote certificate authorities to have a stable comparison with the reconciled data
		sort.SliceStable(remoteCAList.Items, func(i, j int) bool {
			// We don't need to compare the namespace because they are all in the same one
			return remoteCAList.Items[i].Name < remoteCAList.Items[j].Name
		})
		remoteCertificateAuthorities = make([][]byte, len(remoteCAList.Items))
		for i, remoteCA := range remoteCAList.Items {
			remoteCertificateAuthorities[i] = remoteCA.Data[certificates.CAFileName]
		}
	} else {
		// if remoteCAList is empty we use the provided transport CA so that we don't end up having an empty cert file mounted on the ES container
		remoteCertificateAuthorities = [][]byte{certificates.EncodePEMCert(transportCA.Cert.Raw)}
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
