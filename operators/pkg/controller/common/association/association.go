// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ElasticsearchAuthSettings returns the user and the password to be used by an associated object to authenticate
// against an Elasticsearch cluster.
func ElasticsearchAuthSettings(
	c k8s.Client,
	associated v1alpha1.Associated,
) (username, password string, err error) {
	auth := associated.ElasticsearchAuth()
	// if auth is provided via a secret, resolve credentials from it.
	if auth.SecretKeyRef != nil {
		secretObjKey := types.NamespacedName{Namespace: associated.GetNamespace(), Name: auth.SecretKeyRef.Name}
		var secret v1.Secret
		if err := c.Get(secretObjKey, &secret); err != nil {
			return "", "", err
		}
		return auth.SecretKeyRef.Key, string(secret.Data[auth.SecretKeyRef.Key]), nil
	}

	// no authentication method provided, return an empty credential
	return "", "", nil
}
