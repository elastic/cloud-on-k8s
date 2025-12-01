package autoops

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
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
		// we don't have a good way of watching for the license level to change so just requeue with a reasonably long delay
		return results.WithRequeue(5 * time.Minute), status
	}

	// run validation in case the webhook is disabled
	if err := r.validate(ctx, &policy); err != nil {
		r.recorder.Eventf(&policy, corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
		return results.WithError(err), status
	}

	// reconcile dynamic watch for secret referenced in the spec
	if err := r.reconcileWatches(policy); err != nil {
		return results.WithError(err), status
	}

	return r.internalReconcile(ctx, policy, results, status)
}

func (r *ReconcileAutoOpsAgentPolicy) internalReconcile(ctx context.Context, policy autoopsv1alpha1.AutoOpsAgentPolicy, results *reconciler.Results, status autoopsv1alpha1.AutoOpsAgentPolicyStatus) (*reconciler.Results, autoopsv1alpha1.AutoOpsAgentPolicyStatus) {
	log := ulog.FromContext(ctx)
	log.V(1).Info("Internal reconcile AutoOpsAgentPolicy")

	// 1. Search for resources matching the ResourceSelector
	// 2. For each resource, check if it is ready
	// 3. If the resource is not ready, requeue with a reasonably long delay
	// 4. If ready, generate expected resources for the autoops deployment
	// 5. reconcile the expected resources with the actual resources

	var esList esv1.ElasticsearchList
	if err := r.Client.List(ctx, &esList, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(policy.Spec.ResourceSelector.MatchLabels),
	}, &client.ListOptions{Namespace: ""}); err != nil {
		return results.WithError(err), status
	}

	for _, es := range esList.Items {
		if es.Status.Phase != esv1.ElasticsearchReadyPhase {
			results = results.WithRequeue(defaultRequeue)
			continue
		}

		// generate expected resources for the autoops deployment
		expectedResources, err := r.generateExpectedResources(policy, es)
		if err != nil {
			return results.WithError(err), status
		}

		// reconcile the expected resources with the actual resources
		if err := r.reconcileExpectedResources(ctx, es, expectedResources); err != nil {
			return results.WithError(err), status
		}
	}

	return results, status
}
