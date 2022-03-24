// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type ServiceAccountToken struct {
	FullyQualifiedServiceAccountName string
	HashedSecret                     string
}

type ServiceAccountTokens []ServiceAccountToken

const (
	ServiceTokensFileName = "service_tokens"

	ServiceAccountTokenNameField = "name"
	ServiceAccountHashField      = "hash"
)

// getServiceAccountToken reads a service account token from a secret.
func getServiceAccountToken(secret corev1.Secret) (ServiceAccountToken, error) {
	token := ServiceAccountToken{}
	if len(secret.Data) == 0 {
		return token, fmt.Errorf("service account token secret %s/%s is empty", secret.Namespace, secret.Name)
	}

	if serviceAccountName, ok := secret.Data[ServiceAccountTokenNameField]; ok && len(serviceAccountName) > 0 {
		token.FullyQualifiedServiceAccountName = string(serviceAccountName)
	} else {
		return token, fmt.Errorf(fieldNotFound, ServiceAccountTokenNameField, secret.Namespace, secret.Name)
	}

	if hash, ok := secret.Data[ServiceAccountHashField]; ok && len(hash) > 0 {
		token.HashedSecret = string(hash)
	} else {
		return token, fmt.Errorf(fieldNotFound, ServiceAccountHashField, secret.Namespace, secret.Name)
	}

	return token, nil
}

func (s ServiceAccountTokens) Add(serviceAccountToken ServiceAccountToken) ServiceAccountTokens {
	return append(s, serviceAccountToken)
}

func (s *ServiceAccountTokens) ToBytes() []byte {
	if s == nil {
		return []byte{}
	}
	// Ensure that the file is sorted for stable comparison.
	sort.SliceStable(*s, func(i, j int) bool {
		return (*s)[i].FullyQualifiedServiceAccountName < (*s)[j].FullyQualifiedServiceAccountName
	})
	var result strings.Builder
	for _, serviceToken := range *s {
		result.WriteString(serviceToken.FullyQualifiedServiceAccountName)
		result.WriteString(":")
		result.WriteString(serviceToken.HashedSecret)
		result.WriteString("\n")
	}
	return []byte(result.String())
}
