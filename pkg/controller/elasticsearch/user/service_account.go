// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package user

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/set"
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

// NamespacedServices extracts the namespaced service accounts.
func (s ServiceAccountTokens) NamespacedServices() set.StringSet {
	result := set.Make()
	for _, saToken := range s {
		nameItems := strings.Split(saToken.FullyQualifiedServiceAccountName, "/")
		if len(nameItems) != 3 {
			// We are expecting 3 items: the namespace, the service name, and the token name.
			// Ignore any entry that does not match this pattern.
			continue
		}
		result.Add(fmt.Sprintf("%s/%s", nameItems[0], nameItems[1]))
	}
	return result
}

func GetServiceAccountTokens(c k8s.Client, es esv1.Elasticsearch) (ServiceAccountTokens, error) {
	// list all associated user secrets
	var serviceAccountSecrets corev1.SecretList
	if err := c.List(context.Background(),
		&serviceAccountSecrets,
		client.InNamespace(es.Namespace),
		client.MatchingLabels(
			map[string]string{
				label.ClusterNameLabelName: es.Name,
				commonv1.TypeLabelName:     ServiceAccountTokenType,
			},
		),
	); err != nil {
		return nil, err
	}

	var tokens ServiceAccountTokens
	for _, secret := range serviceAccountSecrets.Items {
		token, err := getServiceAccountToken(secret)
		if err != nil {
			return nil, err
		}
		tokens = tokens.Add(token)
	}
	return tokens, nil
}

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
