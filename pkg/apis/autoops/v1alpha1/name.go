// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package v1alpha1

import (
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	common_name "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
)

var (
	// AutoOpsNamer is a Namer that is configured with the defaults for resources related to an AutoOps agent.a
	AutoOpsNamer = common_name.NewNamer("autoops")
)

const (
	deploymentSuffix   = "deploy"
	configSuffix       = "config"
	caSecretSuffix     = "ca-secret"      //nolint:gosec
	apiKeySecretSuffix = "api-key-secret" //nolint:gosec
)

// Deployment returns the name of the deployment for the given policy and ES instance.
func Deployment(policyName string, es esv1.Elasticsearch) string {
	hash := hash.HashObject(es.GetNamespace() + es.GetName())
	return AutoOpsNamer.Suffix(policyName, deploymentSuffix, hash)
}

// Config returns the name of the ConfigMap which holds the AutoOps Agent configuration for the given policy and ES instance.
func Config(policyName string, es esv1.Elasticsearch) string {
	hash := hash.HashObject(es.GetNamespace() + es.GetName())
	return AutoOpsNamer.Suffix(policyName, configSuffix, hash)
}

// CASecret returns the name of the Secret which holds the Elasticsearch CA certificate for the given policy and ES instance.
func CASecret(policyName string, es esv1.Elasticsearch) string {
	hash := hash.HashObject(es.GetNamespace() + es.GetName())
	return AutoOpsNamer.Suffix(policyName, caSecretSuffix, hash)
}

// APIKeySecret returns the name of the Secret which holds the Elasticsearch API key for the given policy and ES instance.
func APIKeySecret(policyName string, es esv1.Elasticsearch) string {
	hash := hash.HashObject(es.GetNamespace() + es.GetName())
	return AutoOpsNamer.Suffix(policyName, apiKeySecretSuffix, hash)
}
