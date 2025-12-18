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
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"

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
		Namespace: policy.Namespace,
		Name:      policy.Spec.AutoOpsRef.SecretName,
	}); err != nil {
		log.Error(err, "while validating configuration secret", "namespace", policy.Namespace, "name", policy.Spec.AutoOpsRef.SecretName)
		state.UpdateInvalidPhaseWithEvent(err.Error())
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

	// Clean up resources that no longer match the Policy's selector
	if err := r.cleanupOrphanedResourcesForPolicy(ctx, log, policy, esList.Items); err != nil {
		log.Error(err, "while cleaning up orphaned resources")
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		results.WithError(err)
	}

	if len(esList.Items) == 0 {
		log.Info("No Elasticsearch resources found for the AutoOpsAgentPolicy")
		state.UpdateWithPhase(autoopsv1alpha1.NoResourcesPhase).
			UpdateResources(len(esList.Items))
		return results
	}

	state.UpdateResources(len(esList.Items))
	readyCount := 0
	errorCount := 0

	for _, es := range esList.Items {
		log := log.WithValues("es_namespace", es.Namespace, "es_name", es.Name)

		// Check if access is allowed via RBAC
		allowed, err := r.accessReviewer.AccessAllowed(
			ctx,
			policy.Spec.ServiceAccountName,
			policy.Namespace,
			&es,
		)
		if err != nil {
			log.Error(err, "while checking access to Elasticsearch resource via RBAC")
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}
		if !allowed {
			log.V(1).Info("Skipping ES cluster - RBAC denied",
				"service_account", policy.Spec.ServiceAccountName,
			)
			continue
		}

		// Continue if RBAC is allowed and ES cluster is ready.
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			log.V(1).Info("Skipping ES cluster that is not ready", "es_namespace", es.Namespace, "es_name", es.Name)
			state.UpdateWithPhase(autoopsv1alpha1.ResourcesNotReadyPhase)
			results = results.WithRequeue(defaultRequeue)
			continue
		}

		if es.Spec.HTTP.TLS.Enabled() {
			if err := r.reconcileAutoOpsESCASecret(ctx, policy, es); err != nil {
				log.Error(err, "while reconciling AutoOps ES CA secret", "es_namespace", es.Namespace, "es_name", es.Name)
				errorCount++
				state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
				results.WithError(err)
				continue
			}
		}

		apiKeySecret, err := r.reconcileAutoOpsESAPIKey(ctx, policy, es)
		if err != nil {
			log.Error(err, "while reconciling AutoOps ES API key", "es_namespace", es.Namespace, "es_name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		configMap, err := ReconcileAutoOpsESConfigMap(ctx, r.Client, policy, es)
		if err != nil {
			log.Error(err, "while reconciling AutoOps ES config map", "es_namespace", es.Namespace, "es_name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		configHash, err := buildConfigHash(ctx, *configMap, *apiKeySecret, r.Client, policy)
		if err != nil {
			log.Error(err, "while building config hash", "es_namespace", es.Namespace, "es_name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		deploymentParams, err := r.buildDeployment(configHash, policy, es)
		if err != nil {
			log.Error(err, "while getting deployment params", "es_namespace", es.Namespace, "es_name", es.Name)
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		reconciledDeployment, err := deployment.Reconcile(ctx, r.Client, deploymentParams, &policy)
		if err != nil {
			log.Error(err, "while reconciling deployment", "es_namespace", es.Namespace, "es_name", es.Name)
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

	return results
}

// cleanupOrphanedResourcesForPolicy removes resources (Deployments, ConfigMaps, Secrets) for ES clusters
// that no longer match the Policy's selector.
func (r *AgentPolicyReconciler) cleanupOrphanedResourcesForPolicy(
	ctx context.Context,
	log logr.Logger,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	clusterMatchingPolicy []esv1.Elasticsearch,
) error {
	// Build a set of ES clusters that should have resources
	// within the cluster.
	esSet := sets.New[types.NamespacedName]()
	for _, es := range clusterMatchingPolicy {
		esSet.Insert(k8s.ExtractNamespacedName(&es))
	}

	matchLabels := client.MatchingLabels{
		PolicyNameLabelKey: policy.Name,
	}

	if err := cleanupOrphanedDeployments(ctx, log, r.Client, policy, matchLabels, esSet); err != nil {
		return fmt.Errorf("while cleaning up deployments: %w", err)
	}

	if err := cleanupOrphanedConfigMaps(ctx, log, r.Client, policy, matchLabels, esSet); err != nil {
		return fmt.Errorf("while cleaning up configmaps: %w", err)
	}

	// Cleanup both CA secrets and API Key.
	if err := cleanupOrphanedSecrets(ctx, log, r.Client, r.esClientProvider, r.params.Dialer, policy, matchLabels, esSet); err != nil {
		return fmt.Errorf("while cleaning up secrets: %w", err)
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
