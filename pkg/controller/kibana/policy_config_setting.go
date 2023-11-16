// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	kibanav1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// PolicyConfig is a structure for storing Kibana config from the StackConfigPolicy
type PolicyConfig struct {
	KibanaConfig      *common.CanonicalConfig
	PolicyAnnotations map[string]string
}

// getPolicyConfig parses the StackConfigPolicy secret and returns a PolicyConfig struct
func getPolicyConfig(ctx context.Context, client k8s.Client, kibana kibanav1.Kibana) (PolicyConfig, error) {
	var policyConfig PolicyConfig
	// Retrieve secret created by the StackConfigPolicy controller if it exists
	// Check for stack config policy Kibana config secret
	stackConfigPolicyConfigSecret := corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      stackconfigpolicy.GetPolicyConfigSecretName(kibana),
		Namespace: kibana.Namespace,
	}, &stackConfigPolicyConfigSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return policyConfig, err
	}

	// Additional annotations to be applied on the Kibana pods
	policyConfig.PolicyAnnotations = map[string]string{
		stackconfigpolicy.KibanaConfigHashAnnotation: stackConfigPolicyConfigSecret.Annotations[stackconfigpolicy.KibanaConfigHashAnnotation],
	}

	// Parse Kibana config from the stack config policy secret.
	var kbConfigFromStackConfigPolicy map[string]interface{}
	if string(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.KibanaConfigKey]) != "" {
		err = json.Unmarshal(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.ElasticSearchConfigKey], &kbConfigFromStackConfigPolicy)
		if err != nil {
			return policyConfig, err
		}
	}
	canonicalConfig, err := common.NewCanonicalConfigFrom(kbConfigFromStackConfigPolicy)
	if err != nil {
		return policyConfig, err
	}
	policyConfig.KibanaConfig = canonicalConfig

	return policyConfig, nil
}
