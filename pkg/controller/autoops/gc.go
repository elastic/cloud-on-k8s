// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// cleanupOrphanedDeployments removes deployments for ES clusters that no longer match the selector.
func cleanupOrphanedDeployments(
	ctx context.Context,
	log logr.Logger,
	c k8s.Client,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	matchLabels client.MatchingLabels,
	matchingESset sets.Set[types.NamespacedName],
) error {
	var deployments appsv1.DeploymentList
	if err := c.List(ctx, &deployments, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		esNN, shouldDelete := shouldDeleteResource(deployment, matchingESset)
		if !shouldDelete {
			continue
		}
		log.Info("Deleting orphaned Deployment", "deployment", deployment.Name, "es_namespace", esNN.Namespace, "es_name", esNN.Name)
		if err := c.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// cleanupOrphanedConfigMaps removes configmaps for ES clusters that no longer match the selector.
func cleanupOrphanedConfigMaps(
	ctx context.Context,
	log logr.Logger,
	c k8s.Client,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	matchLabels client.MatchingLabels,
	matchingESset sets.Set[types.NamespacedName],
) error {
	var configMaps corev1.ConfigMapList
	if err := c.List(ctx, &configMaps, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range configMaps.Items {
		configMap := &configMaps.Items[i]
		esNN, shouldDelete := shouldDeleteResource(configMap, matchingESset)
		if !shouldDelete {
			continue
		}
		log.Info("Deleting orphaned ConfigMap", "configmap", configMap.Name, "es_namespace", esNN.Namespace, "es_name", esNN.Name)
		if err := c.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// cleanupOrphanedSecrets removes secrets (CA and API key) for ES clusters that no longer match the selector.
func cleanupOrphanedSecrets(
	ctx context.Context,
	log logr.Logger,
	c k8s.Client,
	esClientProvider commonesclient.Provider,
	dialer net.Dialer,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	matchLabels client.MatchingLabels,
	matchingESset sets.Set[types.NamespacedName],
) error {
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]
		esNN, shouldDelete := shouldDeleteResource(secret, matchingESset)
		if !shouldDelete {
			continue
		}
		// Check if this is an API key secret
		if secretType, exists := secret.Labels[policySecretTypeLabelKey]; exists && secretType == "api-key" {
			// Try to get the ES cluster to clean up API key
			var es esv1.Elasticsearch
			if err := c.Get(ctx, esNN, &es); err == nil {
				// ES cluster exists, try to clean up API key
				if err := cleanupAutoOpsESAPIKey(ctx, c, esClientProvider, dialer, policy.Namespace, policy.Name, es); err != nil {
					log.Error(err, "while cleaning up API key for ES cluster, continuing with secret deletion", "es_namespace", esNN.Namespace, "es_name", esNN.Name)
				}
			}
		}

		log.Info("Deleting orphaned Secret", "secret", secret.Name, "es_namespace", esNN.Namespace, "es_name", esNN.Name)
		if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// shouldDeleteResource determines if a resource should be deleted based on whether its ES cluster
// matches the current selector. Returns the ES cluster namespaced name and whether to delete.
func shouldDeleteResource(
	resource metav1.Object,
	esSet sets.Set[types.NamespacedName],
) (types.NamespacedName, bool) {
	labels := resource.GetLabels()
	esName, hasESName := labels[commonapikey.MetadataKeyESName]
	esNamespace, hasESNamespace := labels[commonapikey.MetadataKeyESNamespace]

	// If the resource doesn't have ES identity labels, don't delete it.
	if !hasESName || !hasESNamespace {
		return types.NamespacedName{}, false
	}

	esNN := types.NamespacedName{Namespace: esNamespace, Name: esName}

	// If the ES cluster is in the matching list, don't delete
	return esNN, !esSet.Has(esNN)
}
