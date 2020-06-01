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

// WriteAssocSecretToConfigHash dereferences auth secret (if any) to include it in the configHash.
func WriteAssocSecretToConfigHash(client k8s.Client, assoc commonv1.Associated, configHash hash.Hash) error {
	assocConf := assoc.AssociationConf()

	if assocConf.AuthIsConfigured() {
		authSecretNsName := types.NamespacedName{
			Name:      assocConf.GetAuthSecretName(),
			Namespace: assoc.GetNamespace()}
		var authSecret corev1.Secret
		if err := client.Get(authSecretNsName, &authSecret); err != nil {
			return err
		}
		_, _ = configHash.Write(authSecret.Data[assocConf.GetAuthSecretKey()])
	}

	if assocConf.CAIsConfigured() {
		publicCASecretNsName := types.NamespacedName{
			Namespace: assoc.GetNamespace(),
			Name:      assocConf.GetCASecretName()}
		var publicCASecret corev1.Secret
		if err := client.Get(publicCASecretNsName, &publicCASecret); err != nil {
			return err
		}
		if certPem, ok := publicCASecret.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(certPem)
		}
	}

	return nil
}
