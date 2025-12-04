// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// Config holds the parsed configuration from the AutoOpsAgentPolicy configuration secret.
type Config struct {
	CCMApiKey      string
	TempResourceID string
	AutoOpsOTelURL string
	AutoOpsToken   string
}

const (
	// Secret key names for the configuration fields
	ccmApiKey      = "cloud-connected-mode-api-key"
	autoOpsOTelURL = "autoops-otel-url"
	autoOpsToken   = "autoops-token"
)

// ParseConfigSecret retrieves and parses the configuration secret referenced in the AutoOpsAgentPolicy.
// It returns a Config struct containing the parsed configuration values and any error encountered.
func ParseConfigSecret(ctx context.Context, client k8s.Client, secretKey types.NamespacedName) (*Config, error) {
	if secretKey.Name == "" {
		return nil, fmt.Errorf("secret name cannot be empty")
	}

	var secret corev1.Secret
	if err := client.Get(ctx, secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("configuration secret %s/%s not found: %w", secretKey.Namespace, secretKey.Name, err)
		}
		return nil, fmt.Errorf("failed to retrieve configuration secret %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}

	return validateAndPopulateConfig(secret, secretKey)
}

func validateAndPopulateConfig(secret corev1.Secret, secretKey types.NamespacedName) (*Config, error) {
	var config Config

	if data, exists := secret.Data[ccmApiKey]; exists && len(data) > 0 {
		config.CCMApiKey = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", ccmApiKey, secretKey.Namespace, secretKey.Name)
	}

	if data, exists := secret.Data[autoOpsOTelURL]; exists && len(data) > 0 {
		config.AutoOpsOTelURL = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", autoOpsOTelURL, secretKey.Namespace, secretKey.Name)
	}

	if data, exists := secret.Data[autoOpsToken]; exists && len(data) > 0 {
		config.AutoOpsToken = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", autoOpsToken, secretKey.Namespace, secretKey.Name)
	}

	return &config, nil
}
