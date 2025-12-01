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
