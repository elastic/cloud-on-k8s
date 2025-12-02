package autoops

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

func (r *ReconcileAutoOpsAgentPolicy) doReconcile(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy) (*reconciler.Results, autoopsv1alpha1.AutoOpsAgentPolicyStatus) {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Reconcile AutoOpsAgentPolicy")

	results := reconciler.NewResult(ctx)
	status := autoopsv1alpha1.NewStatus(policy)

	// Enterprise license check
	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err), status
	}
	if !enabled {
		msg := "AutoOpsAgentPolicy is an enterprise feature. Enterprise features are disabled"
		log.Info(msg)
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReconciliationError, msg)
		status.Phase = autoopsv1alpha1.InvalidPhase
		return results.WithRequeue(5 * time.Minute), status
	}

	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &policy); err != nil {
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
		status.Phase = autoopsv1alpha1.InvalidPhase
		return results.WithError(err), status
	}

	// reconcile dynamic watch for secret referenced in the spec
	if err := r.reconcileWatches(policy); err != nil {
		status.Phase = autoopsv1alpha1.ErrorPhase
		return results.WithError(err), status
	}

	result := r.internalReconcile(ctx, policy, results, &status)
	return result, status
}

func (r *ReconcileAutoOpsAgentPolicy) internalReconcile(
	ctx context.Context,
	policy autoopsv1alpha1.AutoOpsAgentPolicy,
	results *reconciler.Results,
	status *autoopsv1alpha1.AutoOpsAgentPolicyStatus) *reconciler.Results {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Internal reconcile AutoOpsAgentPolicy")

	// prepare the selector to find resources.
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels:      policy.Spec.ResourceSelector.MatchLabels,
		MatchExpressions: policy.Spec.ResourceSelector.MatchExpressions,
	})
	if err != nil {
		status.Phase = autoopsv1alpha1.ErrorPhase
		return results.WithError(err)
	}
	listOpts := client.ListOptions{LabelSelector: selector}

	// restrict the search to the policy namespace if it is different from the operator namespace
	log.V(1).Info("comparing policy namespace with operator namespace", "policy namespace", policy.Namespace, "operator namespace", r.params.OperatorNamespace)
	if policy.Namespace != r.params.OperatorNamespace {
		log.V(1).Info("Restricting search to policy namespace", "namespace", policy.Namespace)
		listOpts.Namespace = policy.Namespace
	}

	var esList esv1.ElasticsearchList
	if err := r.Client.List(ctx, &esList, &listOpts); err != nil {
		status.Phase = autoopsv1alpha1.ErrorPhase
		return results.WithError(err)
	}

	if len(esList.Items) == 0 {
		log.Info("No Elasticsearch resources found for the AutoOpsAgentPolicy", "namespace", policy.Namespace, "name", policy.Name)
		status.Phase = autoopsv1alpha1.NoResourcesPhase
		status.Resources = len(esList.Items)
		return results
	}

	if err := reconcileAutoOpsESPasswordsSecret(ctx, r.Client, policy, esList.Items); err != nil {
		status.Phase = autoopsv1alpha1.ErrorPhase
		return results.WithError(err)
	}

	for _, es := range esList.Items {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			results = results.WithRequeue(defaultRequeue)
			continue
		}

		if err := ReconcileAutoOpsESConfigMap(ctx, r.Client, policy); err != nil {
			status.Phase = autoopsv1alpha1.ErrorPhase
			return results.WithError(err)
		}

		expectedResources, err := r.generateExpectedResources(policy, es)
		if err != nil {
			status.Phase = autoopsv1alpha1.ErrorPhase
			return results.WithError(err)
		}

		_, err = deployment.Reconcile(ctx, r.Client, expectedResources.deployment, &policy)
		if err != nil {
			status.Phase = autoopsv1alpha1.ErrorPhase
			return results.WithError(err)
		}
	}

	status.Phase = autoopsv1alpha1.ReadyPhase
	return results
}
