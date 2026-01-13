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
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
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

	namespacesFilter, err := k8s.NamespaceFilterFunc(ctx, r.Client, policy.Spec.NamespaceSelector)
	if err != nil {
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

	// Filter ES clusters based on RBAC access before cleanup and reconciliation.
	// This ensures resources are cleaned up when access is revoked.
	accessibleClusters := make([]esv1.Elasticsearch, 0, len(esList.Items))
	for _, es := range esList.Items {
		// filter by namespace (if set)
		if !namespacesFilter(es.Namespace) {
			log.V(1).Info("Skipping ES cluster due to namespace filtering", "es_namespace", es.Namespace, "es_name", es.Name)
			continue
		}

		// filter by RBAC (if set)
		accessAllowed, err := isAutoOpsAssociationAllowed(ctx, r.accessReviewer, &policy, &es, r.recorder)
		if err != nil {
			log.Error(err, "while checking access for Elasticsearch cluster", "es_namespace", es.Namespace, "es_name", es.Name)
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}
		if accessAllowed {
			accessibleClusters = append(accessibleClusters, es)
		} else {
			log.V(1).Info("Skipping ES cluster due to access denied", "es_namespace", es.Namespace, "es_name", es.Name)
		}
	}

	// Clean up resources that no longer match the Policy's selector OR where access was revoked
	if err := r.cleanupOrphanedResourcesForPolicy(ctx, log, policy, accessibleClusters); err != nil {
		log.Error(err, "while cleaning up orphaned resources", "policy_namespace", policy.Namespace, "policy_name", policy.Name)
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		results.WithError(err)
	}

	if len(accessibleClusters) == 0 {
		log.Info("No accessible Elasticsearch resources found for the AutoOpsAgentPolicy")
		state.UpdateWithPhase(autoopsv1alpha1.NoResourcesPhase).
			UpdateResources(0)
		return results
	}

	state.UpdateResources(len(accessibleClusters))
	readyCount := 0
	errorCount := 0

	for _, es := range accessibleClusters {
		log := log.WithValues("es_namespace", es.Namespace, "es_name", es.Name)

		esVersion, err := version.Parse(es.Spec.Version)
		if err != nil {
			log.Error(err, "while parsing ES version")
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			return results.WithError(err)
		}

		// No error means the version is within the deprecated range, so we skip the cluster.
		// We do not adjust the status to indicate this issue at this time, as the status object
		// does not currently support a status per-cluster.
		if version.DeprecatedVersions.WithinRange(esVersion) == nil {
			log.Info("Skipping ES cluster because of deprecated version", "version", es.Spec.Version)
			continue
		}

		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			log.V(1).Info("Skipping ES cluster that is not ready")
			state.UpdateWithPhase(autoopsv1alpha1.ResourcesNotReadyPhase)
			results = results.WithRequeue(reconciler.DefaultRequeue)
			continue
		}

		if es.Spec.HTTP.TLS.Enabled() {
			if err := r.reconcileAutoOpsESCASecret(ctx, policy, es); err != nil {
				log.Error(err, "while reconciling AutoOps ES CA secret")
				errorCount++
				state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
				results.WithError(err)
				continue
			}
		}

		apiKeySecret, err := r.reconcileAutoOpsESAPIKey(ctx, policy, es)
		if err != nil {
			log.Error(err, "while reconciling AutoOps ES API key")
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		configMap, err := ReconcileAutoOpsESConfigMap(ctx, r.Client, policy, es)
		if err != nil {
			log.Error(err, "while reconciling AutoOps ES config map")
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		configHash, err := buildConfigHash(ctx, *configMap, *apiKeySecret, r.Client, policy)
		if err != nil {
			log.Error(err, "while building config hash")
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		deploymentParams, err := r.buildDeployment(configHash, policy, es)
		if err != nil {
			log.Error(err, "while getting deployment params")
			errorCount++
			state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
			results.WithError(err)
			continue
		}

		reconciledDeployment, err := deployment.Reconcile(ctx, r.Client, deploymentParams, &policy)
		if err != nil {
			log.Error(err, "while reconciling deployment")
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

	// Schedule a requeue to periodically re-check RBAC permissions.
	// Use ReconciliationComplete() to indicate this is a periodic check, not an incomplete reconciliation.
	if rbacResult := association.RequeueRbacCheck(r.accessReviewer); rbacResult.RequeueAfter > 0 {
		results = results.WithReconciliationState(
			reconciler.RequeueAfter(rbacResult.RequeueAfter).ReconciliationComplete(),
		)
	}

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
