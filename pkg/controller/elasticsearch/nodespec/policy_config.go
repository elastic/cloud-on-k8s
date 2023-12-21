// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

// PolicyConfig is a structure for storing Elasticsearch config from the StackConfigPolicy
type PolicyConfig struct {
	ElasticsearchConfig *common.CanonicalConfig
	PolicyAnnotations   map[string]string
	AdditionalVolumes   []volume.VolumeLike
}

// getPolicyConfig parses the StackConfigPolicy secret and returns a PolicyConfig struct
func getPolicyConfig(ctx context.Context, client k8s.Client, es esv1.Elasticsearch) (PolicyConfig, error) {
	var policyConfig PolicyConfig
	// Retrieve secret created by the StackConfigPolicy controller if it exists
	// Check for stack config policy Elasticsearch config secret
	stackConfigPolicyConfigSecret := corev1.Secret{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      esv1.StackConfigElasticsearchConfigSecretName(es.Name),
		Namespace: es.Namespace,
	}, &stackConfigPolicyConfigSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return policyConfig, err
	}

	// Additional annotations to be applied on the Elasticsearch pods
	policyConfig.PolicyAnnotations = map[string]string{
		commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: stackConfigPolicyConfigSecret.Annotations[commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation],
	}

	// Parse Elasticsearch config from the stack config policy secret.
	var esConfigFromStackConfigPolicy map[string]interface{}
	if string(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.ElasticSearchConfigKey]) != "" {
		err = json.Unmarshal(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.ElasticSearchConfigKey], &esConfigFromStackConfigPolicy)
		if err != nil {
			return policyConfig, err
		}
	}
	canonicalConfig, err := common.NewCanonicalConfigFrom(esConfigFromStackConfigPolicy)
	if err != nil {
		return policyConfig, err
	}
	policyConfig.ElasticsearchConfig = canonicalConfig

	// Construct additional mounts for the Elasticsearch pods
	var additionalSecretMounts []policyv1alpha1.SecretMount
	if string(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.SecretsMountKey]) != "" {
		if err := json.Unmarshal(stackConfigPolicyConfigSecret.Data[stackconfigpolicy.SecretsMountKey], &additionalSecretMounts); err != nil {
			return policyConfig, err
		}
	}
	for _, secretMount := range additionalSecretMounts {
		secretName := esv1.StackConfigAdditionalSecretName(es.Name, secretMount.SecretName)
		secretVolumeFromStackConfigPolicy := volume.NewSecretVolumeWithMountPath(secretName, secretName, secretMount.MountPath)
		policyConfig.AdditionalVolumes = append(policyConfig.AdditionalVolumes, secretVolumeFromStackConfigPolicy)
	}

	return policyConfig, nil
}
