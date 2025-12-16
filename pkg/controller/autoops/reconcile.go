// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

func (r *AgentPolicyReconciler) doReconcile(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, state *State) *reconciler.Results {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile AutoOpsAgentPolicy")

	results := reconciler.NewResult(ctx)

	// Enterprise license check
	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err)
	}
	if !enabled {
		msg := "AutoOpsAgentPolicy is an enterprise feature. Enterprise features are disabled"
		log.Info(msg)
		state.UpdateInvalidPhaseWithEvent(msg)
		return results.WithRequeue(5 * time.Minute)
	}

	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &policy); err != nil {
		state.UpdateInvalidPhaseWithEvent(err.Error())
		return results.WithError(err)
	}

	// reconcile dynamic watch for secret referenced in the spec
	if err := r.reconcileWatches(policy); err != nil {
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		return results.WithError(err)
	}

	return r.internalReconcile(ctx, policy, results, state)
}

func (r *AgentPolicyReconciler) internalReconcile(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	results *reconciler.Results,
	state *State,
) *reconciler.Results {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Internal reconcile AutoOpsAgentPolicy")

	if err := validateConfigSecret(ctx, r.Client, types.NamespacedName{
		Namespace: autoOpsConfigurationSecretNamespace(policy),
		Name:      policy.Spec.AutoOpsRef.SecretName,
	}); err != nil {
		log.Error(err, "Error validating configuration secret", "namespace", autoOpsConfigurationSecretNamespace(policy), "name", policy.Spec.AutoOpsRef.SecretName)
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		return results.WithError(err)
	}

	// prepare the selector to find resources.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels:      policy.Spec.ResourceSelector.MatchLabels,
		MatchExpressions: policy.Spec.ResourceSelector.MatchExpressions,
	})
	if err != nil {
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		return results.WithError(err)
	}
	listOpts := client.ListOptions{LabelSelector: selector}

	var esList esv1.ElasticsearchList
	if err := r.Client.List(ctx, &esList, &listOpts); err != nil {
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		return results.WithError(err)
	}

	if len(esList.Items) == 0 {
		log.Info("No Elasticsearch resources found for the AutoOpsAgentPolicy", "namespace", policy.Namespace, "name", policy.Name)
		state.UpdateWithPhase(autoopsv1alpha1.NoResourcesPhase).
			UpdateResources(len(esList.Items))
		return results
	}

	state.UpdateResources(len(esList.Items))
	readyCount := 0
	errorCount := 0

	for _, es := range esList.Items {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			log.V(1).Info("Skipping ES cluster that is not ready", "namespace", es.Namespace, "name", es.Name)
			state.UpdateWithPhase(autoopsv1alpha1.ResourcesNotReadyPhase)
			results = results.WithRequeue(defaultRequeue)
			continue
		}

		if es.Spec.HTTP.TLS.Enabled() {
			if err := r.reconcileAutoOpsESCASecret(ctx, policy, es); err != nil {
				log.Error(err, "Error reconciling AutoOps ES CA secret", "namespace", es.Namespace, "name", es.Name)
				errorCount++
				state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
				results.WithError(err)
				continue
			}
		}

		if err := r.reconcileAutoOpsESAPIKey(ctx, policy, es); err != nil {
			log.Error(err, "Error reconciling AutoOps ES API key", "namespace", es.Namespace, "name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		if err := ReconcileAutoOpsESConfigMap(ctx, r.Client, policy, es); err != nil {
			log.Error(err, "Error reconciling AutoOps ES config map", "namespace", es.Namespace, "name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		deploymentParams, err := r.deploymentParams(ctx, policy, es)
		if err != nil {
			log.Error(err, "Error getting deployment params", "namespace", es.Namespace, "name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		reconciledDeployment, err := deployment.Reconcile(ctx, r.Client, deploymentParams, &policy)
		if err != nil {
			log.Error(err, "Error reconciling deployment", "namespace", es.Namespace, "name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		if isDeploymentReady(reconciledDeployment) {
			readyCount++
		}
	}

	state.UpdateReady(readyCount).
		UpdateErrors(errorCount)

	// Clean up resources that no longer match the Policy's selector
	if err := r.cleanupOrphanedResourcesForPolicy(ctx, policy, esList.Items); err != nil {
		log.Error(err, "Error cleaning up orphaned resources")
		// Note: Should we update phase to error, and return error I wonder?
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		results.WithError(err)
	}

	return results
}

// cleanupOrphanedResourcesForPolicy removes resources (Deployments, ConfigMaps, Secrets) for ES clusters
// that no longer match the Policy's selector.
func (r *AgentPolicyReconciler) cleanupOrphanedResourcesForPolicy(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	clusterMatchingPolicy []esv1.Elasticsearch,
) error {
	log := ulog.FromContext(ctx).WithValues("policy", policy.Name, "namespace", policy.Namespace)
	log.V(1).Info("Cleaning up orphaned resources for policy")

	// Build a set of ES clusters that should have resources
	// within the cluster.
	esMap := make(map[types.NamespacedName]struct{})
	for _, es := range clusterMatchingPolicy {
		esMap[k8s.ExtractNamespacedName(&es)] = struct{}{}
	}

	matchLabels := client.MatchingLabels{
		autoOpsLabelName: policy.Name,
	}

	if err := cleanupOrphanedDeployments(ctx, log, r.Client, policy, matchLabels, esMap); err != nil {
		return fmt.Errorf("failed to cleanup deployments: %w", err)
	}

	if err := cleanupOrphanedConfigMaps(ctx, log, r.Client, policy, matchLabels, esMap); err != nil {
		return fmt.Errorf("failed to cleanup configmaps: %w", err)
	}

	// Cleanup both CA secrets and API Key.
	if err := cleanupOrphanedSecrets(ctx, log, r.Client, r.esClientProvider, r.params.Dialer, policy, matchLabels, esMap); err != nil {
		return fmt.Errorf("failed to cleanup secrets: %w", err)
	}

	return nil
}

// isDeploymentReady checks if a deployment is ready by verifying that the DeploymentAvailable condition is true.
func isDeploymentReady(dep appsv1.Deployment) bool {
	for _, condition := range dep.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
