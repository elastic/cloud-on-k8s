// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ParseStackConfigPolicyElasticsearchConfig decodes the Elasticsearch config
// payload from a StackConfigPolicy secret entry into CanonicalConfig.
// Empty payloads return an empty CanonicalConfig.
func ParseStackConfigPolicyElasticsearchConfig(rawConfig []byte) (*common.CanonicalConfig, error) {
	if len(rawConfig) == 0 {
		return common.NewCanonicalConfigFrom(map[string]any{})
	}

	var policyConfig map[string]any
	if err := json.Unmarshal(rawConfig, &policyConfig); err != nil {
		return nil, err
	}

	return common.NewCanonicalConfigFrom(policyConfig)
}

// GetStackConfigPolicyElasticsearchConfig returns the Elasticsearch config from
// the StackConfigPolicy per-cluster secret. Missing secrets return nil config
// and nil error.
func GetStackConfigPolicyElasticsearchConfig(ctx context.Context, c k8s.Client, es esv1.Elasticsearch) (*common.CanonicalConfig, error) {
	var policySecret corev1.Secret
	err := c.Get(ctx, types.NamespacedName{
		Name:      esv1.StackConfigElasticsearchConfigSecretName(es.Name),
		Namespace: es.Namespace,
	}, &policySecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return ParseStackConfigPolicyElasticsearchConfig(policySecret.Data[esv1.StackConfigElasticsearchConfigKey])
}
