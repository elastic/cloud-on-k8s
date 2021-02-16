// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"context"
	"hash"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WriteAssocsToConfigHash dereferences auth and CA secrets (if any) to include it in the configHash.
func WriteAssocsToConfigHash(client k8s.Client, associations []commonv1.Association, configHash hash.Hash) error {
	for _, assoc := range associations {
		if err := writeAuthSecretToConfigHash(client, assoc, configHash); err != nil {
			return err
		}
		if err := writeCASecretToConfigHash(client, assoc, configHash); err != nil {
			return err
		}
	}
	return nil
}

func writeAuthSecretToConfigHash(client k8s.Client, assoc commonv1.Association, configHash hash.Hash) error {
	assocConf := assoc.AssociationConf()
	if !assocConf.AuthIsConfigured() {
		return nil
	}

	authSecretNsName := types.NamespacedName{
		Name:      assocConf.GetAuthSecretName(),
		Namespace: assoc.GetNamespace()}
	var authSecret corev1.Secret
	if err := client.Get(context.Background(), authSecretNsName, &authSecret); err != nil {
		return err
	}
	authSecretData, ok := authSecret.Data[assocConf.GetAuthSecretKey()]
	if !ok {
		return errors.Errorf("auth secret key %s doesn't exist", assocConf.GetAuthSecretKey())
	}

	_, _ = configHash.Write(authSecretData)

	return nil
}

func writeCASecretToConfigHash(client k8s.Client, assoc commonv1.Association, configHash hash.Hash) error {
	assocConf := assoc.AssociationConf()
	if !assocConf.CAIsConfigured() {
		return nil
	}

	publicCASecretNsName := types.NamespacedName{
		Namespace: assoc.GetNamespace(),
		Name:      assocConf.GetCASecretName()}
	var publicCASecret corev1.Secret
	if err := client.Get(context.Background(), publicCASecretNsName, &publicCASecret); err != nil {
		return err
	}
	certPem, ok := publicCASecret.Data[certificates.CertFileName]
	if !ok {
		return errors.Errorf("public CA secret key %s doesn't exist", certificates.CertFileName)
	}

	_, _ = configHash.Write(certPem)

	return nil
}
