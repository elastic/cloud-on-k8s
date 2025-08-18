// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"encoding/json"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/filesettings"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ElasticSearchConfigKey = "elasticsearch.json"
	SecretsMountKey        = "secretMounts.json"
)

func newElasticsearchConfigSecret(policy policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (corev1.Secret, error) {
	data := make(map[string][]byte)
	if len(policy.Spec.Elasticsearch.SecretMounts) > 0 {
		secretMountBytes, err := json.Marshal(policy.Spec.Elasticsearch.SecretMounts)
		if err != nil {
			return corev1.Secret{}, err
		}
		data[SecretsMountKey] = secretMountBytes
	}

	elasticsearchAndMountsConfigHash := getElasticsearchConfigAndMountsHash(policy.Spec.Elasticsearch.Config, policy.Spec.Elasticsearch.SecretMounts)
	if policy.Spec.Elasticsearch.Config != nil {
		configDataJSONBytes, err := policy.Spec.Elasticsearch.Config.MarshalJSON()
		if err != nil {
			return corev1.Secret{}, err
		}
		data[ElasticSearchConfigKey] = configDataJSONBytes
	}
	meta := metadata.Propagate(&es, metadata.Metadata{
		Labels: eslabel.NewLabels(k8s.ExtractNamespacedName(&es)),
		Annotations: map[string]string{
			commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: elasticsearchAndMountsConfigHash,
		},
	})
	elasticsearchConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   es.Namespace,
			Name:        esv1.StackConfigElasticsearchConfigSecretName(es.Name),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: data,
	}

	// Set StackConfigPolicy as the soft owner
	filesettings.SetSoftOwner(&elasticsearchConfigSecret, policy)

	// Add label to delete secret on deletion of the stack config policy
	elasticsearchConfigSecret.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

	return elasticsearchConfigSecret, nil
}

// reconcileSecretMounts creates the secrets in SecretMounts to the respective Elasticsearch namespace where they should be mounted to.
func reconcileSecretMounts(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, policy *policyv1alpha1.StackConfigPolicy, meta metadata.Metadata) error {
	for _, secretMount := range policy.Spec.Elasticsearch.SecretMounts {
		additionalSecret := corev1.Secret{}
		namespacedName := types.NamespacedName{
			Name:      secretMount.SecretName,
			Namespace: policy.Namespace,
		}
		if err := c.Get(ctx, namespacedName, &additionalSecret); err != nil {
			return err
		}

		meta = meta.Merge(metadata.Metadata{
			Annotations: map[string]string{
				commonannotation.SourceSecretAnnotationName: secretMount.SecretName,
			},
		})
		// Recreate it in the Elasticsearch namespace, prefix with es name.
		secretName := esv1.StackConfigAdditionalSecretName(es.Name, secretMount.SecretName)
		expected := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   es.Namespace,
				Name:        secretName,
				Labels:      meta.Labels,
				Annotations: meta.Annotations,
			},
			Data: additionalSecret.Data,
		}

		// Set stackconfigpolicy as a softowner
		filesettings.SetSoftOwner(&expected, *policy)

		// Set the secret to be deleted when the stack config policy is deleted.
		expected.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

		if _, err := reconciler.ReconcileSecret(ctx, c, expected, nil); err != nil {
			return err
		}
	}
	return nil
}

func getElasticsearchConfigAndMountsHash(elasticsearchConfig *commonv1.Config, secretMounts []policyv1alpha1.SecretMount) string {
	if elasticsearchConfig != nil {
		return hash.HashObject([]interface{}{elasticsearchConfig, secretMounts})
	}
	return hash.HashObject(secretMounts)
}

// elasticsearchConfigAndSecretMountsApplied checks if the Elasticsearch config and secret mounts from the stack config policy have been applied to the Elasticsearch cluster.
func elasticsearchConfigAndSecretMountsApplied(ctx context.Context, c k8s.Client, policy policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (bool, error) {
	// Get Pods for the given Elasticsearch
	podList := corev1.PodList{}
	if err := c.List(ctx, &podList, client.InNamespace(es.Namespace), client.MatchingLabels{
		eslabel.ClusterNameLabelName: es.Name,
	}); err != nil || len(podList.Items) == 0 {
		return false, err
	}

	elasticsearchAndMountsConfigHash := getElasticsearchConfigAndMountsHash(policy.Spec.Elasticsearch.Config, policy.Spec.Elasticsearch.SecretMounts)
	for _, esPod := range podList.Items {
		if esPod.Annotations[commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation] != elasticsearchAndMountsConfigHash {
			return false, nil
		}
	}

	return true, nil
}

// Multi-policy versions of the above functions

// newElasticsearchConfigSecretFromPolicies creates an Elasticsearch config secret from multiple policies
func (r *ReconcileStackConfigPolicy) newElasticsearchConfigSecretFromPolicies(policies []policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (corev1.Secret, error) {
	data := make(map[string][]byte)
	var allSecretMounts []policyv1alpha1.SecretMount
	var mergedConfig *commonv1.Config
	
	// Sort policies by weight (descending) so lower weights override higher ones
	sortedPolicies := make([]policyv1alpha1.StackConfigPolicy, len(policies))
	copy(sortedPolicies, policies)
	for i := 0; i < len(sortedPolicies)-1; i++ {
		for j := 0; j < len(sortedPolicies)-i-1; j++ {
			if sortedPolicies[j].Spec.Weight < sortedPolicies[j+1].Spec.Weight {
				sortedPolicies[j], sortedPolicies[j+1] = sortedPolicies[j+1], sortedPolicies[j]
			}
		}
	}
	
	// Merge secret mounts from all policies
	for _, policy := range sortedPolicies {
		allSecretMounts = append(allSecretMounts, policy.Spec.Elasticsearch.SecretMounts...)
		
		// Merge Elasticsearch configs (lower weight policies override higher ones)
		if policy.Spec.Elasticsearch.Config != nil {
			if mergedConfig == nil {
				mergedConfig = policy.Spec.Elasticsearch.Config.DeepCopy()
			} else {
				// Merge the config data, with current policy taking precedence
				for key, value := range policy.Spec.Elasticsearch.Config.Data {
					mergedConfig.Data[key] = value
				}
			}
		}
	}
	
	// Add secret mounts to data if any exist
	if len(allSecretMounts) > 0 {
		secretMountBytes, err := json.Marshal(allSecretMounts)
		if err != nil {
			return corev1.Secret{}, err
		}
		data[SecretsMountKey] = secretMountBytes
	}

	// Add merged config to data if it exists
	elasticsearchAndMountsConfigHash := getElasticsearchConfigAndMountsHash(mergedConfig, allSecretMounts)
	if mergedConfig != nil {
		configDataJSONBytes, err := mergedConfig.MarshalJSON()
		if err != nil {
			return corev1.Secret{}, err
		}
		data[ElasticSearchConfigKey] = configDataJSONBytes
	}

	meta := metadata.Propagate(&es, metadata.Metadata{
		Labels: eslabel.NewLabels(k8s.ExtractNamespacedName(&es)),
		Annotations: map[string]string{
			commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation: elasticsearchAndMountsConfigHash,
		},
	})
	elasticsearchConfigSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   es.Namespace,
			Name:        esv1.StackConfigElasticsearchConfigSecretName(es.Name),
			Labels:      meta.Labels,
			Annotations: meta.Annotations,
		},
		Data: data,
	}

	// Store all policy references in the secret
	var policyRefs []filesettings.PolicyRef
	for _, policy := range policies {
		policyRefs = append(policyRefs, filesettings.PolicyRef{
			Name:      policy.Name,
			Namespace: policy.Namespace,
			Weight:    policy.Spec.Weight,
		})
	}
	if err := filesettings.SetPolicyRefs(&elasticsearchConfigSecret, policyRefs); err != nil {
		return corev1.Secret{}, err
	}

	// Add label to delete secret on deletion of stack config policies
	elasticsearchConfigSecret.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

	return elasticsearchConfigSecret, nil
}

// reconcileSecretMountsFromPolicies creates secrets from all policies' SecretMounts
func (r *ReconcileStackConfigPolicy) reconcileSecretMountsFromPolicies(ctx context.Context, es esv1.Elasticsearch, policies []policyv1alpha1.StackConfigPolicy, meta metadata.Metadata) error {
	// Collect all unique secret mounts from all policies
	secretMountMap := make(map[string]policyv1alpha1.SecretMount) // key is secretName to avoid duplicates
	for _, policy := range policies {
		for _, secretMount := range policy.Spec.Elasticsearch.SecretMounts {
			secretMountMap[secretMount.SecretName] = secretMount
		}
	}

	for _, secretMount := range secretMountMap {
		// Find the policy that contains this secret mount (use the first one found for namespace)
		var sourcePolicy *policyv1alpha1.StackConfigPolicy
		for _, policy := range policies {
			for _, mount := range policy.Spec.Elasticsearch.SecretMounts {
				if mount.SecretName == secretMount.SecretName {
					sourcePolicy = &policy
					break
				}
			}
			if sourcePolicy != nil {
				break
			}
		}

		if sourcePolicy == nil {
			continue
		}

		additionalSecret := corev1.Secret{}
		namespacedName := types.NamespacedName{
			Name:      secretMount.SecretName,
			Namespace: sourcePolicy.Namespace,
		}
		if err := r.Client.Get(ctx, namespacedName, &additionalSecret); err != nil {
			return err
		}

		secretMeta := meta.Merge(metadata.Metadata{
			Annotations: map[string]string{
				commonannotation.SourceSecretAnnotationName: secretMount.SecretName,
			},
		})
		
		// Recreate it in the Elasticsearch namespace, prefix with es name.
		secretName := esv1.StackConfigAdditionalSecretName(es.Name, secretMount.SecretName)
		expected := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   es.Namespace,
				Name:        secretName,
				Labels:      secretMeta.Labels,
				Annotations: secretMeta.Annotations,
			},
			Data: additionalSecret.Data,
		}

		// Store policy references that use this secret mount
		var policyRefs []filesettings.PolicyRef
		for _, policy := range policies {
			for _, mount := range policy.Spec.Elasticsearch.SecretMounts {
				if mount.SecretName == secretMount.SecretName {
					policyRefs = append(policyRefs, filesettings.PolicyRef{
						Name:      policy.Name,
						Namespace: policy.Namespace,
						Weight:    policy.Spec.Weight,
					})
					break
				}
			}
		}
		if err := filesettings.SetPolicyRefs(&expected, policyRefs); err != nil {
			return err
		}

		// Set the secret to be deleted when stack config policies are deleted
		expected.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

		if _, err := reconciler.ReconcileSecret(ctx, r.Client, expected, nil); err != nil {
			return err
		}
	}
	return nil
}

// elasticsearchConfigAndSecretMountsAppliedFromPolicies checks if configs from all policies have been applied
func (r *ReconcileStackConfigPolicy) elasticsearchConfigAndSecretMountsAppliedFromPolicies(ctx context.Context, policies []policyv1alpha1.StackConfigPolicy, es esv1.Elasticsearch) (bool, error) {
	// Get Pods for the given Elasticsearch
	podList := corev1.PodList{}
	if err := r.Client.List(ctx, &podList, client.InNamespace(es.Namespace), client.MatchingLabels{
		eslabel.ClusterNameLabelName: es.Name,
	}); err != nil || len(podList.Items) == 0 {
		return false, err
	}

	// Compute expected hash from merged policies
	var allSecretMounts []policyv1alpha1.SecretMount
	var mergedConfig *commonv1.Config

	// Sort policies by weight and merge (descending order)
	sortedPolicies := make([]policyv1alpha1.StackConfigPolicy, len(policies))
	copy(sortedPolicies, policies)
	for i := 0; i < len(sortedPolicies)-1; i++ {
		for j := 0; j < len(sortedPolicies)-i-1; j++ {
			if sortedPolicies[j].Spec.Weight < sortedPolicies[j+1].Spec.Weight {
				sortedPolicies[j], sortedPolicies[j+1] = sortedPolicies[j+1], sortedPolicies[j]
			}
		}
	}

	for _, policy := range sortedPolicies {
		allSecretMounts = append(allSecretMounts, policy.Spec.Elasticsearch.SecretMounts...)
		if policy.Spec.Elasticsearch.Config != nil {
			if mergedConfig == nil {
				mergedConfig = policy.Spec.Elasticsearch.Config.DeepCopy()
			} else {
				for key, value := range policy.Spec.Elasticsearch.Config.Data {
					mergedConfig.Data[key] = value
				}
			}
		}
	}

	expectedHash := getElasticsearchConfigAndMountsHash(mergedConfig, allSecretMounts)
	for _, esPod := range podList.Items {
		if esPod.Annotations[commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation] != expectedHash {
			return false, nil
		}
	}

	return true, nil
}
