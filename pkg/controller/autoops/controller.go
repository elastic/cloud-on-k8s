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

// Add creates a new AutoOpsAgentPolicy controller and adds it to the manager with default RBAC. The manager will set fields on the controller
// and start it when the manager is started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

func newReconciler(mgr manager.Manager, params operator.Parameters) *AgentPolicyReconciler {
	k8sClient := mgr.GetClient()
	return &AgentPolicyReconciler{
		Client:           k8sClient,
		recorder:         mgr.GetEventRecorderFor(controllerName),
		licenseChecker:   license.NewLicenseChecker(k8sClient, params.OperatorNamespace),
		params:           params,
		dynamicWatches:   watches.NewDynamicWatches(),
		esClientProvider: commonesclient.NewClient,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *AgentPolicyReconciler) error {
	// watch for changes to AutoOpsAgentPolicy
	if err := c.Watch(source.Kind(mgr.GetCache(), &autoopsv1alpha1.AutoOpsAgentPolicy{}, &handler.TypedEnqueueRequestForObject[*autoopsv1alpha1.AutoOpsAgentPolicy]{})); err != nil {
		return err
	}

	// watch for changes to Elasticsearch and reconcile all AutoOpsAgentPolicies
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &esv1.Elasticsearch{}, reconcileRequestForAllAutoOpsPolicies(r.Client))); err != nil {
		return err
	}

	// watch dynamically referenced secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

// reconcileRequestForAllAutoOpsPolicies returns the requests to reconcile all AutoOpsAgentPolicy resources.
func reconcileRequestForAllAutoOpsPolicies(clnt k8s.Client) handler.TypedEventHandler[client.Object, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, es client.Object) []reconcile.Request {
		var autoOpsAgentPolicyList autoopsv1alpha1.AutoOpsAgentPolicyList
		err := clnt.List(context.Background(), &autoOpsAgentPolicyList)
		if err != nil {
			ulog.Log.Error(err, "Fail to list AutoOpsAgentPolicyList while watching Elasticsearch")
			return nil
		}
		requests := make([]reconcile.Request, 0)
		for _, autoOpsAgentPolicy := range autoOpsAgentPolicyList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&autoOpsAgentPolicy)})
		}
		return requests
	})
}

var _ reconcile.Reconciler = (*AgentPolicyReconciler)(nil)

// AgentPolicyReconciler reconciles an AutoOpsAgentPolicy object
type AgentPolicyReconciler struct {
	k8s.Client
	recorder         record.EventRecorder
	licenseChecker   license.Checker
	params           operator.Parameters
	dynamicWatches   watches.DynamicWatches
	esClientProvider commonesclient.Provider
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reconciles the AutoOpsAgentPolicy resource ensuring that any resources are created/updated/deleted as needed.
func (r *AgentPolicyReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "autoops_name", request)
	log := ulog.FromContext(ctx).WithValues(
		"policy_namespace", request.Namespace,
		"policy_name", request.Name,
	)
	defer common.LogReconciliationRun(log)()
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

	if common.IsUnmanaged(ctx, &policy) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	state := newState(policy)
	if policy.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&policy))
	}

	results := r.doReconcile(ctx, policy, state)
	updatePhaseFromResults(results, state)

	result, err := r.updateStatusFromState(ctx, state)
	if err != nil {
		results.WithError(err)
	} else if result.RequeueAfter > 0 {
		return results.WithRequeue().Aggregate()
	}

	return results.Aggregate()
}

// updatePhaseFromResults updates the phase of the AutoOpsAgentPolicy status based on the results of the reconciliation
func updatePhaseFromResults(results *reconciler.Results, state *State) {
	if isReconciled, message := results.IsReconciled(); !isReconciled {
		state.UpdateWithPhase(autoopsv1alpha1.ApplyingChangesPhase)
		state.AddEvent(corev1.EventTypeWarning, events.EventReasonDelayed, message)
	} else {
		state.UpdateWithPhase(autoopsv1alpha1.ReadyPhase)
	}
}

func (r *AgentPolicyReconciler) validate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy) error {
	span, ctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := policy.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, policy, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(ctx, err)
	}

	return nil
}

func (r *AgentPolicyReconciler) updateStatusFromState(ctx context.Context, state *State) (reconcile.Result, error) {
	span, ctx := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	events, policy := state.Apply()
	for _, evt := range events {
		ulog.FromContext(ctx).V(1).Info("Recording event", "event", evt)
		r.recorder.Event(&state.policy, evt.EventType, evt.Reason, evt.Message)
	}
	if policy == nil {
		ulog.FromContext(ctx).V(1).Info("Status is up to date", "iteration", atomic.LoadUint64(&r.iteration))
		return reconcile.Result{}, nil
	}

	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"status", policy.Status,
	)
	if err := common.UpdateStatus(ctx, r.Client, policy); err != nil {
		if apierrors.IsConflict(err) {
			return reconcile.Result{RequeueAfter: defaultRequeue}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}
	return reconcile.Result{}, nil
}

func (r *AgentPolicyReconciler) onDelete(ctx context.Context, obj types.NamespacedName) error {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx)
	log.Info("Cleaning up AutoOpsAgentPolicy resources")

	// Remove dynamic watches on secrets
	r.dynamicWatches.Secrets.RemoveHandlerForKey(configSecretWatchName(obj))

	// Cleanup API keys for all Elasticsearch clusters that match this policy.
	// Query for secrets labeled with this policy to find all associated ES clusters.
	var secrets corev1.SecretList
	matchLabels := client.MatchingLabels{
		PolicyNameLabelKey:      obj.Name,
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
			// On any error, still attempt to delete the API key secret.
			if err := deleteESAPIKeySecret(ctx, r.Client, log,
				types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name},
				types.NamespacedName{Namespace: esNamespace, Name: esName}); err != nil {
				log.Error(err, "Failed to delete API key secret", "es_namespace", esNamespace, "es_name", esName)
			}
			continue
		}

		// This cleanup requires communicating with Elasticsearch so we do not attempt this is the previous retrival of the ES cluster fails.
		if err := cleanupAutoOpsESAPIKey(ctx, r.Client, r.esClientProvider, r.params.Dialer, obj.Namespace, obj.Name, es); err != nil {
			log.Error(err, "Failed to cleanup API key for Elasticsearch cluster", "es_namespace", esNamespace, "es_name", esName)
			continue
		}
		log.V(1).Info("Successfully cleaned up API key", "es_namespace", esNamespace, "es_name", esName)
	}

	return nil
}

// reconcileWatches sets up dynamic watches for secrets referenced in the AutoOpsAgentPolicy spec.
func (r *AgentPolicyReconciler) reconcileWatches(policy autoopsv1alpha1.AutoOpsAgentPolicy) error {
	watcher := k8s.ExtractNamespacedName(&policy)

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
