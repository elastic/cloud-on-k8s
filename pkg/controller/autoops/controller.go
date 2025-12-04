// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
)

const (
	controllerName = "autoops-controller"
)

// defaultRequeue is the default requeue interval for this controller.
var defaultRequeue = 5 * time.Second

// Add creates a new AutoOpsAgentPolicy Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// newReconciler returns a new reconcile.Reconciler of AutoOpsAgentPolicy.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileAutoOpsAgentPolicy {
	k8sClient := mgr.GetClient()
	return &ReconcileAutoOpsAgentPolicy{
		Client:           k8sClient,
		recorder:         mgr.GetEventRecorderFor(controllerName),
		licenseChecker:   license.NewLicenseChecker(k8sClient, params.OperatorNamespace),
		params:           params,
		dynamicWatches:   watches.NewDynamicWatches(),
		esClientProvider: commonesclient.NewClient,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileAutoOpsAgentPolicy) error {
	// watch for changes to AutoOpsAgentPolicy
	if err := c.Watch(source.Kind(mgr.GetCache(), &autoopsv1alpha1.AutoOpsAgentPolicy{}, &handler.TypedEnqueueRequestForObject[*autoopsv1alpha1.AutoOpsAgentPolicy]{})); err != nil {
		return err
	}

	// watch dynamically referenced secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcileAutoOpsAgentPolicy{}

// ReconcileAutoOpsAgentPolicy reconciles an AutoOpsAgentPolicy object
type ReconcileAutoOpsAgentPolicy struct {
	k8s.Client
	recorder         record.EventRecorder
	licenseChecker   license.Checker
	params           operator.Parameters
	dynamicWatches   watches.DynamicWatches
	esClientProvider commonesclient.Provider
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reconciles the AutoOpsAgentPolicy resource ensuring that any deployments are created/updated/deleted as needed.
func (r *ReconcileAutoOpsAgentPolicy) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "autoops_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the AutoOpsAgentPolicy resource
	var policy autoopsv1alpha1.AutoOpsAgentPolicy
	err := r.Client.Get(ctx, request.NamespacedName, &policy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	state := NewState(policy)
	results := reconciler.NewResult(ctx)

	_, err = ParseConfigSecret(ctx, r.Client, types.NamespacedName{
		Namespace: policy.Namespace,
		Name:      policy.Spec.Config.SecretRef.SecretName,
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			state.UpdateInvalidPhaseWithEvent("Config secret not found")
			// update status before returning
			if err := r.updateStatusFromState(ctx, state); err != nil {
				if apierrors.IsConflict(err) {
					return reconcile.Result{Requeue: true, RequeueAfter: defaultRequeue}, nil
				}
				return reconcile.Result{}, tracing.CaptureError(ctx, err)
			}
			return reconcile.Result{Requeue: true, RequeueAfter: defaultRequeue}, nil
		}
		state.UpdateWithPhase(autoopsv1alpha1.ErrorPhase)
		// update status before returning
		if err := r.updateStatusFromState(ctx, state); err != nil {
			if apierrors.IsConflict(err) {
				return reconcile.Result{Requeue: true, RequeueAfter: defaultRequeue}, nil
			}
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &policy) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if policy.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&policy))
	}

	results = r.doReconcile(ctx, policy, state)

	if state.status.Phase != autoopsv1alpha1.InvalidPhase && state.status.Phase != autoopsv1alpha1.ErrorPhase {
		if isReconciled, _ := results.IsReconciled(); !isReconciled {
			state.UpdateWithPhase(autoopsv1alpha1.ApplyingChangesPhase)
		} else {
			state.UpdateWithPhase(autoopsv1alpha1.ReadyPhase)
		}
	}

	if err := r.updateStatusFromState(ctx, state); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithRequeue().Aggregate()
		}
		results.WithError(err)
	}

	return results.Aggregate()
}

func (r *ReconcileAutoOpsAgentPolicy) validate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := policy.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, policy, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileAutoOpsAgentPolicy) updateStatusFromState(ctx context.Context, state *State) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	events, policy := state.Apply()
	for _, evt := range events {
		ulog.FromContext(ctx).V(1).Info("Recording event", "event", evt)
		r.recorder.Event(&state.policy, evt.EventType, evt.Reason, evt.Message)
	}
	if policy == nil {
		ulog.FromContext(ctx).V(1).Info("Status is up to date", "iteration", atomic.LoadUint64(&r.iteration))
		return nil
	}

	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"status", policy.Status,
	)
	return common.UpdateStatus(ctx, r.Client, policy)
}

func (r *ReconcileAutoOpsAgentPolicy) onDelete(ctx context.Context, obj types.NamespacedName) error {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx).WithValues("policy_namespace", obj.Namespace, "policy_name", obj.Name)
	log.Info("Cleaning up AutoOpsAgentPolicy resources")

	// Remove dynamic watches on secrets
	r.dynamicWatches.Secrets.RemoveHandlerForKey(configSecretWatchName(obj))

	// Cleanup API keys for all Elasticsearch clusters that match this policy.
	// Query for secrets labeled with this policy to find all associated ES clusters.
	var secrets corev1.SecretList
	matchLabels := client.MatchingLabels{
		policyNameLabelKey:      obj.Name,
		policyNamespaceLabelKey: obj.Namespace,
	}
	if err := r.Client.List(ctx, &secrets, matchLabels); err != nil {
		return tracing.CaptureError(ctx, fmt.Errorf("while listing secrets for policy %s/%s: %w", obj.Namespace, obj.Name, err))
	}

	// Cleanup API keys for each ES cluster referenced by the secrets
	for _, secret := range secrets.Items {
		esName, hasESName := secret.Labels["elasticsearch.k8s.elastic.co/name"]
		esNamespace, hasESNamespace := secret.Labels["elasticsearch.k8s.elastic.co/namespace"]
		if !hasESName || !hasESNamespace {
			log.V(1).Info("Secret missing ES cluster labels, skipping", "secret", secret.Name)
			continue
		}

		var es esv1.Elasticsearch
		if err := r.Client.Get(ctx, types.NamespacedName{Namespace: esNamespace, Name: esName}, &es); err != nil {
			if apierrors.IsNotFound(err) {
				log.V(1).Info("Elasticsearch cluster not found, skipping cleanup", "es_namespace", esNamespace, "es_name", esName)
				continue
			}
			log.Error(err, "Failed to get Elasticsearch cluster", "es_namespace", esNamespace, "es_name", esName)
			continue
		}

		if err := cleanupAutoOpsESAPIKey(ctx, r.Client, r.esClientProvider, r.params.Dialer, obj.Namespace, obj.Name, es); err != nil {
			log.Error(err, "Failed to cleanup API key for Elasticsearch cluster", "es_namespace", esNamespace, "es_name", esName)
			continue
		}
		log.V(1).Info("Successfully cleaned up API key", "es_namespace", esNamespace, "es_name", esName)
	}

	return nil
}

// reconcileWatches sets up dynamic watches for secrets referenced in the AutoOpsAgentPolicy spec.
func (r *ReconcileAutoOpsAgentPolicy) reconcileWatches(policy autoopsv1alpha1.AutoOpsAgentPolicy) error {
	watcher := types.NamespacedName{
		Name:      policy.Name,
		Namespace: policy.Namespace,
	}

	secretNames := []string{policy.Spec.Config.SecretRef.SecretName}

	// Set up dynamic watches for referenced secrets
	return watches.WatchUserProvidedSecrets(
		watcher,
		r.dynamicWatches,
		configSecretWatchName(watcher),
		secretNames,
	)
}

// configSecretWatchName returns the name of the watch registered on secrets referenced in the config.
func configSecretWatchName(watcher types.NamespacedName) string {
	return fmt.Sprintf("%s-%s-config-secret", watcher.Namespace, watcher.Name)
}
