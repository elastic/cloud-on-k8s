// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfig

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	common "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	essettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/stackconfigpolicy"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// PolicyConfig is a structure for storing Elasticsearch config from the StackConfigPolicy
type PolicyConfig struct {
	ElasticsearchConfig *common.CanonicalConfig
	PolicyAnnotations   map[string]string
	AdditionalVolumes   []volume.VolumeLike
	// Roles holds Elasticsearch role definitions provided through the StackConfigPolicy.
	// The key is the role name, the value is the role spec.
	Roles user.RolesFileContent
	// RolesHash is the hash of the SCP-provided roles, used to track when ES has applied them.
	RolesHash string
}

// GetPolicyConfig parses the StackConfigPolicy secret and returns a PolicyConfig.
func GetPolicyConfig(ctx context.Context, client k8s.Client, es esv1.Elasticsearch) (PolicyConfig, error) {
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
	canonicalConfig, err := essettings.ParseStackConfigPolicyElasticsearchConfig(
		stackConfigPolicyConfigSecret.Data[esv1.StackConfigElasticsearchConfigKey],
	)
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

	// Parse SCP-provided role definitions and propagate the roles hash annotation.
	policyConfig.RolesHash = stackConfigPolicyConfigSecret.Annotations[commonannotation.ElasticsearchRolesHashAnnotation]
	if rolesData := stackConfigPolicyConfigSecret.Data[esv1.StackConfigRolesKey]; len(rolesData) > 0 {
		var roles user.RolesFileContent
		if err := json.Unmarshal(rolesData, &roles); err != nil {
			return policyConfig, err
		}
		policyConfig.Roles = roles
	}

	return policyConfig, nil
}

// UserRoles returns the SCP-derived role definitions and their hash.
func (c PolicyConfig) UserRoles() user.PolicyRoles {
	return user.PolicyRoles{Roles: c.Roles, Hash: c.RolesHash}
}
