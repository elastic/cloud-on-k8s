// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	ifs "github.com/elastic/cloud-on-k8s/pkg/controller/common/interfaces"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ElasticsearchAuthSettings returns the user and the password to be used by an associated object to authenticate
// against an Elasticsearch cluster.
func ElasticsearchAuthSettings(
	c k8s.Client,
	associated ifs.Associated,
) (username, password string, err error) {
	assocConf := associated.AssociationConf()
	if !assocConf.AuthIsConfigured() {
		return "", "", nil
	}

	secretObjKey := types.NamespacedName{Namespace: associated.GetNamespace(), Name: assocConf.AuthSecretName}
	var secret v1.Secret
	if err := c.Get(secretObjKey, &secret); err != nil {
		return "", "", err
	}
	return assocConf.AuthSecretKey, string(secret.Data[assocConf.AuthSecretKey]), nil
}
