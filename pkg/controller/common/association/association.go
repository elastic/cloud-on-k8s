// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"hash"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// WriteAssocSecretToHash dereferences auth secret (if any) to include it in the checksum
func WriteAssocSecretToHash(client k8s.Client, assoc commonv1.Associated, hash hash.Hash) error {
	assocConf := assoc.AssociationConf()

	if assocConf.AuthIsConfigured() {
		esAuthKey := types.NamespacedName{
			Name:      assocConf.GetAuthSecretName(),
			Namespace: assoc.GetNamespace()}
		var esAuthSecret corev1.Secret
		if err := client.Get(esAuthKey, &esAuthSecret); err != nil {
			return err
		}
		_, _ = hash.Write(esAuthSecret.Data[assocConf.GetAuthSecretKey()])
	}

	if assocConf.CAIsConfigured() {
		esPublicCAKey := types.NamespacedName{
			Namespace: assoc.GetNamespace(),
			Name:      assocConf.GetCASecretName()}
		var esPublicCASecret corev1.Secret
		if err := client.Get(esPublicCAKey, &esPublicCASecret); err != nil {
			return err
		}
		if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
			_, _ = hash.Write(certPem)
		}
	}

	return nil
}
