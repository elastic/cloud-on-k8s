// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/go-logr/logr"
	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolsevents "k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	autoopsvalidation "github.com/elastic/cloud-on-k8s/v3/pkg/controller/autoops/validation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	commonesclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/esclient"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

const (
	controllerName = "autoops-controller"
)

// Add creates a new AutoOpsAgentPolicy controller and adds it to the manager with default RBAC. The manager will set fields on the controller
// and start it when the manager is started.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := newReconciler(mgr, accessReviewer, params)
	c, err := common.NewNamespacedController(mgr, controllerName, r, params, namespaceFlipRequests(ulog.Log, mgr.GetCache()))
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

func newReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *AgentPolicyReconciler {
	k8sClient := mgr.GetClient()
	return &AgentPolicyReconciler{
		Client:           k8sClient,
		accessReviewer:   accessReviewer,
		recorder:         mgr.GetEventRecorder(controllerName),
		params:           params,
		dynamicWatches:   watches.NewDynamicWatches(),
		esClientProvider: commonesclient.NewClient,
		licenseChecker:   license.NewLicenseChecker(k8sClient, params.OperatorNamespace),
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *AgentPolicyReconciler) error {
	m := r.params.NamespaceMatcher
	// watch for changes to AutoOpsAgentPolicy
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &autoopsv1alpha1.AutoOpsAgentPolicy{}, &handler.TypedEnqueueRequestForObject[*autoopsv1alpha1.AutoOpsAgentPolicy]{})); err != nil {
		return err
	}

	// watch for changes to Elasticsearch and reconcile all AutoOpsAgentPolicies
	if err := c.Watch(watches.NamespacedKind[client.Object](m, mgr.GetCache(), &esv1.Elasticsearch{}, reconcileRequestForAllAutoOpsPolicies(r.Client))); err != nil {
		return err
	}

	// watch dynamically referenced secrets
	if err := c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets)); err != nil {
		return err
	}

	// watch for changes to deployments created by this controller
	return c.Watch(watches.NamespacedKind(m, mgr.GetCache(), &appsv1.Deployment{}, reconcileRequestForAutoOpsPolicyFromDeployment()))
}

// namespaceFlipRequests returns a mapper translating a namespace match-state change into
// reconcile requests for the AutoOpsAgentPolicies affected by it. A policy selects its
// Elasticsearch clusters cluster-wide by label (ResourceSelector), so a flip of any namespace
// can change any policy's set of matching clusters: policies living in the flipped namespace
// must be picked up or cleaned up, and policies living elsewhere must deploy agents for newly
// scoped clusters or clean up agents and API keys for de-scoped ones. Mirroring the
// Elasticsearch watch above (reconcileRequestForAllAutoOpsPolicies), all policies are
// re-enqueued.
func namespaceFlipRequests(log logr.Logger, cache cache.Cache) func(context.Context, *corev1.Namespace) []reconcile.Request {
	return func(ctx context.Context, ns *corev1.Namespace) []reconcile.Request {
		var list autoopsv1alpha1.AutoOpsAgentPolicyList
		// List **cluster-wide** from the cache (not the FilterClient): policies in the
		// namespace being de-scoped would be hidden by the FilterClient, causing us to miss
		// the reconcile requests needed to clean them up.
		if err := cache.List(ctx, &list); err != nil {
			log.Error(err, "failed to list AutoOpsAgentPolicies in namespace flip watch", "namespace", ns.Name)
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(list.Items))
		for i := range list.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&list.Items[i])})
		}
		return reqs
	}
}

// reconcileRequestForAllAutoOpsPolicies returns the requests to reconcile all AutoOpsAgentPolicy resources.
func reconcileRequestForAllAutoOpsPolicies(clnt k8s.Client) handler.TypedEventHandler[client.Object, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, es client.Object) []reconcile.Request {
		var autoOpsAgentPolicyList autoopsv1alpha1.AutoOpsAgentPolicyList
		err := clnt.List(context.Background(), &autoOpsAgentPolicyList)
		if err != nil {
			ulog.Log.Error(err, "failed to list AutoOpsAgentPolicyList while watching Elasticsearch")
			return nil
		}
		requests := make([]reconcile.Request, 0)
		for _, autoOpsAgentPolicy := range autoOpsAgentPolicyList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(&autoOpsAgentPolicy)})
		}
		return requests
	})
}

// reconcileRequestForAutoOpsPolicyFromDeployment returns a handler that enqueues the AutoOpsAgentPolicy
// associated with a deployment based on the deployment's labels.
func reconcileRequestForAutoOpsPolicyFromDeployment() handler.TypedEventHandler[*appsv1.Deployment, reconcile.Request] {
	return handler.TypedEnqueueRequestsFromMapFunc(func(ctx context.Context, dep *appsv1.Deployment) []reconcile.Request {
		deploymentNN := policyFromLabels(dep.GetLabels())

		if deploymentNN.Name == "" {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: deploymentNN,
			},
		}
	})
}

var _ reconcile.Reconciler = (*AgentPolicyReconciler)(nil)
var _ driver.Interface = (*AgentPolicyReconciler)(nil)

// AgentPolicyReconciler reconciles an AutoOpsAgentPolicy object
type AgentPolicyReconciler struct {
	k8s.Client
	accessReviewer   rbac.AccessReviewer
	recorder         toolsevents.EventRecorder
	params           operator.Parameters
	dynamicWatches   watches.DynamicWatches
	esClientProvider commonesclient.Provider
	licenseChecker   license.Checker
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reconciles the AutoOpsAgentPolicy resource ensuring that any resources are created/updated/deleted as needed.
func (r *AgentPolicyReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.params.Tracer, controllerName, "policy_name", request)
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

	if common.IsUnmanaged(ctx, &policy) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	state := newState(policy)
	if policy.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&policy))
	}

	results := r.doReconcile(ctx, policy, state)
	state.Finalize(results.IsReconciled())

	result, err := r.updateStatusFromState(ctx, state)
	results = results.WithResult(result).WithError(err)

	// requeue if the phase is in the set of phases that require a requeue
	if state.status.Phase.NeedsRequeue() {
		return results.WithRequeue(reconciler.DefaultRequeue).Aggregate()
	}

	return results.Aggregate()
}

func (r *AgentPolicyReconciler) validate(ctx context.Context, policy *autoopsv1alpha1.AutoOpsAgentPolicy) error {
	span, ctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if err := autoopsvalidation.Validate(ctx, policy, r.licenseChecker); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, policy, events.EventReasonValidation, events.EventActionValidation, err.Error())
		return tracing.CaptureError(ctx, err)
	}

	return nil
}

func (r *AgentPolicyReconciler) updateStatusFromState(ctx context.Context, state *State) (reconcile.Result, error) {
	span, ctx := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()
	log := ulog.FromContext(ctx)

	events, policy := state.Apply()
	for _, evt := range events {
		log.V(1).Info("Recording event", "event", evt)
		k8s.EmitEvent(r.recorder, &state.policy, evt.EventType, evt.Reason, evt.Action, evt.Message)
	}
	if policy == nil {
		log.V(1).Info("Status is up to date", "iteration", atomic.LoadUint64(&r.iteration))
		return reconcile.Result{}, nil
	}

	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"status", policy.Status,
	)
	if err := common.UpdateStatus(ctx, r.Client, policy); err != nil {
		if apierrors.IsConflict(err) {
			return reconcile.Result{RequeueAfter: reconciler.DefaultRequeue}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}
	return reconcile.Result{}, nil
}

// OnNamespaceOutOfScope releases all controller-local state associated with the given AutoOpsAgentPolicy
// resource when its namespace no longer matches the operator's namespace selector.
func (r *AgentPolicyReconciler) OnNamespaceOutOfScope(obj types.NamespacedName) {
	r.dynamicWatches.Secrets.RemoveHandlerForKey(configSecretWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
}

func (r *AgentPolicyReconciler) onDelete(ctx context.Context, obj types.NamespacedName) error {
	defer tracing.Span(&ctx)()
	log := ulog.FromContext(ctx)
	log.Info("Cleaning up AutoOpsAgentPolicy resources")

	r.OnNamespaceOutOfScope(obj)

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
		// Remove dynamic watch registered for this secret (CA or API key)
		// Safety-net for in-scope secrets at policy deletion,
		// selector-change cleanup is handled in cleanupOrphanedSecrets
		r.dynamicWatches.Secrets.RemoveHandlerForKey(secret.Name)

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
				log.Error(err, "while deleting API key secret", "es_namespace", esNamespace, "es_name", esName)
			}
			continue
		}

		// This cleanup requires communicating with Elasticsearch so we do not attempt this if the previous retrieval of the ES cluster fails.
		if err := cleanupAutoOpsESAPIKey(ctx, r.Client, r.esClientProvider, r.params.Dialer, obj.Namespace, obj.Name, es); err != nil {
			log.Error(err, "while cleaning up API key for Elasticsearch cluster", "es_namespace", esNamespace, "es_name", esName)
			continue
		}
		log.V(1).Info("Successfully cleaned up API key", "es_namespace", esNamespace, "es_name", esName)
	}

	return nil
}

// reconcileWatches sets up dynamic watches for secrets referenced in the AutoOpsAgentPolicy spec.
func (r *AgentPolicyReconciler) reconcileWatches(policy autoopsv1alpha1.AutoOpsAgentPolicy) error {
	watcher := k8s.ExtractNamespacedName(&policy)

	secretNames := []string{policy.Spec.AutoOpsRef.SecretName}

	// Set up dynamic watch for the AutoOpsRef secret.
	// The configRef watch is managed by common.ParseConfigRef, called unconditionally
	// in ReconcileAutoOpsESConfigMap, so we do not set it up here.
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

// K8sClient returns the Kubernetes client from the reconciler, satisfying driver.Interface.
func (r *AgentPolicyReconciler) K8sClient() k8s.Client { return r.Client }

// DynamicWatches returns the set of dynamic watches from the reconciler, satisfying driver.Interface.
func (r *AgentPolicyReconciler) DynamicWatches() watches.DynamicWatches { return r.dynamicWatches }

// Recorder returns the event recorder from the reconciler, satisfying driver.Interface.
func (r *AgentPolicyReconciler) Recorder() toolsevents.EventRecorder { return r.recorder }
