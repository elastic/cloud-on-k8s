// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"encoding/json"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	commonannotation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	commonlabels "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ElasticSearchConfigKey = "elasticsearch.json"
	SecretsMountKey        = "secretMounts.json"
)

func newElasticsearchConfigSecret(esConfig policyv1alpha1.ElasticsearchConfigPolicySpec, es esv1.Elasticsearch) (corev1.Secret, error) {
	data := make(map[string][]byte)
	if len(esConfig.SecretMounts) > 0 {
		secretMountBytes, err := json.Marshal(esConfig.SecretMounts)
		if err != nil {
			return corev1.Secret{}, err
		}
		data[SecretsMountKey] = secretMountBytes
	}

	elasticsearchAndMountsConfigHash := getElasticsearchConfigAndMountsHash(esConfig.Config, esConfig.SecretMounts)
	if esConfig.Config != nil {
		configDataJSONBytes, err := esConfig.Config.MarshalJSON()
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

	// Add label to delete secret on deletion of the stack config policy
	elasticsearchConfigSecret.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

	return elasticsearchConfigSecret, nil
}

// reconcileSecretMounts creates the secrets in SecretMounts to the respective Elasticsearch namespace where they should be mounted to.
// It also removes any previously mounted secrets that are no longer in policy.Spec.Elasticsearch.SecretMounts.
func reconcileSecretMounts(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, policy *policyv1alpha1.StackConfigPolicy, meta metadata.Metadata) error {
	// Track which secret mounts are defined in the policy (by their target secret name in ES namespace)
	secretMountsInPolicy := make(sets.Set[string], len(policy.Spec.Elasticsearch.SecretMounts))

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
		setSingleSoftOwner(&expected, *policy)

		// Set the secret to be deleted when the stack config policy is deleted.
		expected.Labels[commonlabels.StackConfigPolicyOnDeleteLabelName] = commonlabels.OrphanSecretDeleteOnPolicyDelete

		if _, err := reconciler.ReconcileSecret(ctx, c, expected, nil); err != nil {
			return err
		}

		secretMountsInPolicy.Insert(secretName)
	}

	// Clean up secret mounts that are no longer in the policy
	return cleanupOrphanedSecretMounts(ctx, c, es, k8s.ExtractNamespacedName(policy), secretMountsInPolicy)
}

// cleanupOrphanedSecretMounts removes copied secrets that are owned by the given policy and do not exist in the given
// secretMountsInPolicy set. This handles the case where a policy is updated in-place and SecretMounts list is affected.
// See https://github.com/elastic/cloud-on-k8s/issues/8921
func cleanupOrphanedSecretMounts(ctx context.Context, c k8s.Client, es esv1.Elasticsearch, policyNsn types.NamespacedName, secretMountsInPolicy sets.Set[string]) error {
	// List all secrets in the ES namespace that are soft-owned by this policy.
	// The label selector below ensures we only find secrets that: (1) are soft-owned
	// by this specific policy, (2) are marked for deletion when the policy is deleted,
	// and (3) belong to this specific ES cluster.
	var secrets corev1.SecretList
	matchLabels := client.MatchingLabels{
		reconciler.SoftOwnerKindLabel:                   policyv1alpha1.Kind,
		reconciler.SoftOwnerNameLabel:                   policyNsn.Name,
		reconciler.SoftOwnerNamespaceLabel:              policyNsn.Namespace,
		commonlabels.StackConfigPolicyOnDeleteLabelName: commonlabels.OrphanSecretDeleteOnPolicyDelete,
		eslabel.ClusterNameLabelName:                    es.Name,
	}

	if err := c.List(ctx, &secrets, client.InNamespace(es.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]

		// Skip secrets that do not have commonannotation.SourceSecretAnnotationName which identifies the ones that
		// were reconciled from a secret mount in the owner StackConfigPolicy. See reconcileSecretMounts func
		if secret.Annotations[commonannotation.SourceSecretAnnotationName] == "" {
			continue
		}

		// Check if this secret is in the expected set (still in SecretMounts)
		if secretMountsInPolicy.Has(secret.Name) {
			continue
		}

		// This secret is owned by the policy but no longer in SecretMounts - delete it
		// Since these are single-owner secrets (filtered by labels), we can delete directly
		if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
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

// elasticsearchConfigAndSecretMountsApplied checks if the Elasticsearch config and secret mounts have been applied to the Elasticsearch cluster.
func elasticsearchConfigAndSecretMountsApplied(ctx context.Context, c k8s.Client, esConfigPolicy policyv1alpha1.ElasticsearchConfigPolicySpec, es esv1.Elasticsearch) (bool, error) {
	// Get Pods for the given Elasticsearch
	podList := corev1.PodList{}
	if err := c.List(ctx, &podList, client.InNamespace(es.Namespace), client.MatchingLabels{
		eslabel.ClusterNameLabelName: es.Name,
	}); err != nil || len(podList.Items) == 0 {
		return false, err
	}

	elasticsearchAndMountsConfigHash := getElasticsearchConfigAndMountsHash(esConfigPolicy.Config, esConfigPolicy.SecretMounts)
	for _, esPod := range podList.Items {
		if esPod.Annotations[commonannotation.ElasticsearchConfigAndSecretMountsHashAnnotation] != elasticsearchAndMountsConfigHash {
			return false, nil
		}
	}

	return true, nil
}
