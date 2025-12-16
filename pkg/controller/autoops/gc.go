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
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonapikey "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/apikey"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

const (
	AllNamespaces = ""
)

// GarbageCollectOrphanedResources removes resources (ConfigMaps, Secrets, Deployments) that were created
// by the AutoOps controller but whose owner Policy no longer exists.
func GarbageCollectOrphanedResources(ctx context.Context, c k8s.Client, managedNamespaces []string) error {
	log := ulog.FromContext(ctx)
	log.Info("Starting garbage collection of orphaned AutoOps resources")

	if len(managedNamespaces) == 0 {
		managedNamespaces = []string{AllNamespaces}
	}

	for _, ns := range managedNamespaces {
		if err := garbageCollectForNamespace(ctx, c, ns); err != nil {
			log.Error(err, "Failed to garbage collect orphaned resources for namespace", "namespace", ns)
			continue
		}
	}

	return nil
}

func garbageCollectForNamespace(ctx context.Context, c k8s.Client, ns string) error {
	log := ulog.FromContext(ctx).WithValues("namespace", ns)
	var policyList autoopsv1alpha1.AutoOpsAgentPolicyList
	if err := c.List(ctx, &policyList, client.InNamespace(ns)); err != nil {
		return err
	}
	// list all secrets in the namespace
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, client.InNamespace(ns), client.MatchingLabels{commonv1.TypeLabelName: autoOpsAgentType}); err != nil {
		return err
	}
	// list all deployments in the namespace
	var deployments appsv1.DeploymentList
	if err := c.List(ctx, &deployments, client.InNamespace(ns), client.MatchingLabels{commonv1.TypeLabelName: autoOpsAgentType}); err != nil {
		return err
	}
	// list all configmaps in the namespace
	var configMaps corev1.ConfigMapList
	if err := c.List(ctx, &configMaps, client.InNamespace(ns), client.MatchingLabels{commonv1.TypeLabelName: autoOpsAgentType}); err != nil {
		return err
	}

	existingPolicies := make(map[string]*autoopsv1alpha1.AutoOpsAgentPolicy)
	for i := range policyList.Items {
		policy := &policyList.Items[i]
		existingPolicies[policy.Name] = policy
	}

	// Garbage collect orphaned secrets
	for i := range secrets.Items {
		secret := &secrets.Items[i]
		if !isOwnedByExistingPolicy(secret, existingPolicies) {
			log.Info("Deleting orphaned Secret", "name", secret.Name)
			if err := c.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	// Garbage collect orphaned deployments
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		if !isOwnedByExistingPolicy(deployment, existingPolicies) {
			log.Info("Deleting orphaned Deployment", "name", deployment.Name)
			if err := c.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	// Garbage collect orphaned configmaps
	for i := range configMaps.Items {
		configMap := &configMaps.Items[i]
		if !isOwnedByExistingPolicy(configMap, existingPolicies) {
			log.Info("Deleting orphaned ConfigMap", "name", configMap.Name)
			if err := c.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// isOwnedByExistingPolicy checks if a resource is owned by any existing policy.
// A resource is considered owned if it has an owner reference to a policy in the existingPolicies map.
func isOwnedByExistingPolicy(resource metav1.Object, existingPolicies map[string]*autoopsv1alpha1.AutoOpsAgentPolicy) bool {
	ownerRefs := resource.GetOwnerReferences()
	for _, ownerRef := range ownerRefs {
		if ownerRef.Kind == autoopsv1alpha1.Kind {
			if policy, exists := existingPolicies[ownerRef.Name]; exists && policy.GetUID() == ownerRef.UID {
				return true
			}
		}
	}
	return false
}

// cleanupOrphanedDeployments removes deployments for ES clusters that no longer match the selector.
func cleanupOrphanedDeployments(
	ctx context.Context,
	log logr.Logger,
	c k8s.Client,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	matchLabels client.MatchingLabels,
	matchingESMap map[types.NamespacedName]struct{},
) error {
	var deployments appsv1.DeploymentList
	if err := c.List(ctx, &deployments, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		esNN, shouldDelete := shouldDeleteResource(deployment, matchingESMap)
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
	matchingESMap map[types.NamespacedName]struct{},
) error {
	var configMaps corev1.ConfigMapList
	if err := c.List(ctx, &configMaps, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range configMaps.Items {
		configMap := &configMaps.Items[i]
		esNN, shouldDelete := shouldDeleteResource(configMap, matchingESMap)
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
	matchingESMap map[types.NamespacedName]struct{},
) error {
	// List all secrets (both CA and API key secrets now use autoOpsLabelName)
	var secrets corev1.SecretList
	if err := c.List(ctx, &secrets, client.InNamespace(policy.Namespace), matchLabels); err != nil {
		return err
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]
		esNN, shouldDelete := shouldDeleteResource(secret, matchingESMap)
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
					log.Error(err, "Failed to cleanup API key for ES cluster, continuing with secret deletion", "es_namespace", esNN.Namespace, "es_name", esNN.Name)
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
	esMap map[types.NamespacedName]struct{},
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
	_, exists := esMap[esNN]

	return esNN, !exists
}
