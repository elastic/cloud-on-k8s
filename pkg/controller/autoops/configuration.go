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
	AutoOpsOTelURL string
	AutoOpsToken   string
}

const (
	// Secret key names for the configuration fields
	ccmAPIKey      = "cloud-connected-mode-api-key"
	autoOpsOTelURL = "autoops-otel-url"
	autoOpsToken   = "autoops-token"
	ccmAPIURL      = "cloud-connected-mode-api-url"
)

// validateConfigSecret validates the configuration secret referenced in the AutoOpsAgentPolicy.
func validateConfigSecret(ctx context.Context, client k8s.Client, secretNSN types.NamespacedName) error {
	if secretNSN.Name == "" {
		return fmt.Errorf("secret name cannot be empty")
	}

	var secret corev1.Secret
	if err := client.Get(ctx, secretNSN, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("configuration secret %s/%s not found: %w", secretNSN.Namespace, secretNSN.Name, err)
		}
		return fmt.Errorf("while retrieving configuration secret %s/%s: %w", secretNSN.Namespace, secretNSN.Name, err)
	}

	return internalValidate(secret)
}

func internalValidate(secret corev1.Secret) error {
	if data, exists := secret.Data[ccmAPIKey]; !exists || len(data) == 0 {
		return fmt.Errorf("missing required key %s in configuration secret %s/%s", ccmAPIKey, secret.Namespace, secret.Name)
	}

	if data, exists := secret.Data[autoOpsOTelURL]; !exists || len(data) == 0 {
		return fmt.Errorf("missing required key %s in configuration secret %s/%s", autoOpsOTelURL, secret.Namespace, secret.Name)
	}

	if data, exists := secret.Data[autoOpsToken]; !exists || len(data) == 0 {
		return fmt.Errorf("missing required key %s in configuration secret %s/%s", autoOpsToken, secret.Namespace, secret.Name)
	}

	return nil
}
