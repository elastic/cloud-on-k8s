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
	secretKeyCCMApiKey      = "ccmApiKey"
	secretKeyTempResourceID = "tempResourceID"
	secretKeyAutoOpsOTelURL = "autoOpsOTelURL"
	secretKeyAutoOpsToken   = "autoOpsToken"
)

// ParseConfigSecret retrieves and parses the configuration secret referenced in the AutoOpsAgentPolicy.
// It returns a Config struct containing the parsed configuration values and an error if encountered.
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

	config := &Config{}

	// Parse ccmApiKey
	if data, exists := secret.Data[secretKeyCCMApiKey]; exists {
		config.CCMApiKey = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", secretKeyCCMApiKey, secretKey.Namespace, secretKey.Name)
	}

	// Parse tempResourceID
	if data, exists := secret.Data[secretKeyTempResourceID]; exists {
		config.TempResourceID = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", secretKeyTempResourceID, secretKey.Namespace, secretKey.Name)
	}

	// Parse autoOpsOTelURL
	if data, exists := secret.Data[secretKeyAutoOpsOTelURL]; exists {
		config.AutoOpsOTelURL = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", secretKeyAutoOpsOTelURL, secretKey.Namespace, secretKey.Name)
	}

	// Parse autoOpsToken
	if data, exists := secret.Data[secretKeyAutoOpsToken]; exists {
		config.AutoOpsToken = string(data)
	} else {
		return nil, fmt.Errorf("missing required key %s in configuration secret %s/%s", secretKeyAutoOpsToken, secretKey.Namespace, secretKey.Name)
	}

	return config, nil
}
