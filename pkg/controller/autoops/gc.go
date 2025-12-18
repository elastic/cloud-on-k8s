// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"

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
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/net"
)

// GarbageCollector allows removing resources created by the AutoOps controller for policies that no longer exist.
// Resources should be deleted as part of the controller's reconciliation loop. But without a Finalizer, nothing
// prevents the AutoOpsAgentPolicy from being removed while the operator is not running.
// This code is intended to be run during startup, before the controllers are started, to detect and delete such
// orphaned resources.
type GarbageCollector struct {
	client           k8s.Client
	esClientProvider commonesclient.Provider
	dialer           net.Dialer
}

// NewGarbageCollector creates a new AutoOps GarbageCollector instance.
// The provided client is expected to be already restricted to managed namespaces via the manager cache.
func NewGarbageCollector(client k8s.Client, esClientProvider commonesclient.Provider, dialer net.Dialer) *GarbageCollector {
	return &GarbageCollector{
		client:           client,
		esClientProvider: esClientProvider,
		dialer:           dialer,
	}
}

// DoGarbageCollection runs the AutoOps garbage collector.
// It finds API key secrets that reference an AutoOpsAgentPolicy that no longer exists,
// invalidates the corresponding API keys in Elasticsearch, and deletes the secrets.
//
// Note: ConfigMaps and Deployments have owner references to the policy, so Kubernetes
// garbage collection handles their cleanup automatically when a policy is deleted.
// API key secrets intentionally have no owner reference to allow cleanup of the ES API key first.
func (gc *GarbageCollector) DoGarbageCollection(ctx context.Context) error {
	log := ulog.FromContext(ctx)
	log.Info("Starting AutoOps garbage collection")

	// Get all existing policies to check against
	existingPolicies, err := gc.listAllPolicies(ctx)
	if err != nil {
		return fmt.Errorf("while listing AutoOpsAgentPolicies: %w", err)
	}

	// Clean up orphaned secrets (including API keys in ES)
	if err := gc.cleanupOrphanedSecretsForDeletedPolicies(ctx, existingPolicies); err != nil {
		return fmt.Errorf("while cleaning up orphaned secrets: %w", err)
	}

	log.Info("AutoOps garbage collection complete")
	return nil
}

// listAllPolicies returns a set of all existing AutoOpsAgentPolicy namespaced names.
// The client is already restricted to managed namespaces via the manager cache.
func (gc *GarbageCollector) listAllPolicies(ctx context.Context) (sets.Set[types.NamespacedName], error) {
	policies := sets.New[types.NamespacedName]()

	var policyList autoopsv1alpha1.AutoOpsAgentPolicyList
	if err := gc.client.List(ctx, &policyList); err != nil {
		return nil, err
	}
	for _, policy := range policyList.Items {
		policies.Insert(k8s.ExtractNamespacedName(&policy))
	}

	return policies, nil
}

// cleanupOrphanedSecretsForDeletedPolicies finds and deletes secrets whose policy no longer exists.
// The client is already restricted to managed namespaces via the manager cache.
func (gc *GarbageCollector) cleanupOrphanedSecretsForDeletedPolicies(
	ctx context.Context,
	existingPolicies sets.Set[types.NamespacedName],
) error {
	log := ulog.FromContext(ctx)
	// List all secrets with the policy label
	var secrets corev1.SecretList
	if err := gc.client.List(ctx, &secrets,
		client.HasLabels{PolicyNameLabelKey, policyNamespaceLabelKey},
	); err != nil {
		return err
	}

	for i := range secrets.Items {
		secret := &secrets.Items[i]
		policyNN := policyFromLabels(secret.Labels)
		if policyNN.Name == "" {
			continue
		}

		// Check if policy still exists
		if existingPolicies.Has(policyNN) {
			continue
		}

		// Policy doesn't exist, clean up
		log.Info("Found orphaned AutoOps secret for deleted policy",
			"secret", secret.Name, "namespace", secret.Namespace,
			"policy_name", policyNN.Name, "policy_namespace", policyNN.Namespace)

		// If this is an API key secret, try to invalidate the ES API key first.
		// Only delete the secret if invalidation succeeds, otherwise keep it to retry on next startup.
		if secretType, exists := secret.Labels[policySecretTypeLabelKey]; exists && secretType == apiKeySecretType {
			esNN := esFromLabels(secret.Labels)
			if esNN.Name != "" {
				var es esv1.Elasticsearch
				err := gc.client.Get(ctx, esNN, &es)
				switch {
				case err == nil:
					// ES cluster exists, try to clean up API key
					if err := cleanupAutoOpsESAPIKey(ctx, gc.client, gc.esClientProvider, gc.dialer, policyNN.Namespace, policyNN.Name, es); err != nil {
						// Keep the secret to retry invalidation on next startup
						log.Error(err, "Failed to invalidate API key, keeping secret to retry on next startup",
							"secret", secret.Name, "es_namespace", esNN.Namespace, "es_name", esNN.Name)
						continue
					}
					log.Info("Invalidated orphaned API key in Elasticsearch",
						"es_namespace", esNN.Namespace, "es_name", esNN.Name)
				case apierrors.IsNotFound(err):
					// ES cluster doesn't exist, safe to delete the secret (API key is gone with the cluster)
					log.Info("ES cluster not found, API key no longer exists",
						"es_namespace", esNN.Namespace, "es_name", esNN.Name)
				default:
					// Unknown error fetching ES cluster, keep secret to retry on next startup
					log.Error(err, "Failed to get ES cluster, keeping secret to retry on next startup",
						"secret", secret.Name, "es_namespace", esNN.Namespace, "es_name", esNN.Name)
					continue
				}
			}
		}

		// Delete the secret
		if err := gc.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		log.Info("Deleted orphaned AutoOps secret", "secret", secret.Name, "namespace", secret.Namespace)
	}
	return nil
}

// policyFromLabels extracts the policy namespaced name from resource labels.
func policyFromLabels(labels map[string]string) types.NamespacedName {
	name, hasName := labels[PolicyNameLabelKey]
	namespace, hasNamespace := labels[policyNamespaceLabelKey]
	if !hasName || !hasNamespace {
		return types.NamespacedName{}
	}
	return types.NamespacedName{Name: name, Namespace: namespace}
}

// esFromLabels extracts the Elasticsearch namespaced name from resource labels.
func esFromLabels(labels map[string]string) types.NamespacedName {
	name, hasName := labels[commonapikey.MetadataKeyESName]
	namespace, hasNamespace := labels[commonapikey.MetadataKeyESNamespace]
	if !hasName || !hasNamespace {
		return types.NamespacedName{}
	}
	return types.NamespacedName{Name: name, Namespace: namespace}
}

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
	esNN := esFromLabels(resource.GetLabels())

	// If the resource doesn't have ES identity labels, don't delete it.
	if esNN.Name == "" {
		return types.NamespacedName{}, false
	}

	// If the ES cluster is in the matching list, don't delete
	return esNN, !esSet.Has(esNN)
}
