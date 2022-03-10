// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
)

const (
	controllerName = "agent-controller"
)

// Add creates a new Agent Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileAgent {
	client := mgr.GetClient()
	return &ReconcileAgent{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

// addWatches adds watches for all resources this controller cares about
func addWatches(c controller.Controller, r *ReconcileAgent) error {
	// Watch for changes to Agent
	if err := c.Watch(&source.Kind{Type: &agentv1alpha1.Agent{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch DaemonSets
	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &agentv1alpha1.Agent{},
	}); err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &agentv1alpha1.Agent{},
	}); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` is correctly reconciled on any change.
	// Watching Deployments or DaemonSets only may lead to missing some events.
	if err := watches.WatchPods(c, NameLabelName); err != nil {
		return err
	}

	// Watch Secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &agentv1alpha1.Agent{},
	}); err != nil {
		return err
	}

	// Watch services - Agent in Fleet mode with Fleet Server enabled configures and exposes a Service
	// for Elastic Agents to connect to.
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &agentv1alpha1.Agent{},
	}); err != nil {
		return err
	}

	// Watch dynamically referenced Secrets
	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets)
}

var _ reconcile.Reconciler = &ReconcileAgent{}

// ReconcileAgent reconciles an Agent object
type ReconcileAgent struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for an Agent object and makes changes based on the state read
// and what is in the Agent.Spec
func (r *ReconcileAgent) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, err error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "agent_name", request)
	defer common.LogReconciliationRunNoSideEffects(logconf.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	agent := &agentv1alpha1.Agent{}
	if err = association.FetchWithAssociations(ctx, r.Client, request, agent); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(request.NamespacedName)
			return reconcile.Result{}, nil
		}
		if agent == nil {
			return reconcile.Result{}, err
		}
		return updateStatus(ctx, *agent, r.Client, newStatus(*agent)).WithError(err).Aggregate()
	}
	logconf.FromContext(ctx).Info("fetchwithassociation did not fail")

	if common.IsUnmanaged(agent) {
		logconf.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if agent.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	result, err = r.doReconcile(ctx, *agent).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, agent, events.EventReconciliationError, "Reconciliation error: %v", err)

	return result, err
}

func (r *ReconcileAgent) doReconcile(ctx context.Context, agent agentv1alpha1.Agent) (results *reconciler.Results) {
	defer tracing.Span(&ctx)()
	var err error
	results = reconciler.NewResult(ctx)
	status := newStatus(agent)

	// defer the updating of status to ensure that the status is updated regardless of the outcome of the reconciliation.
	// note that this deferred function is modifying the return values, which are named return values, which allows this
	// to function properly.
	defer func() {
		if updateStatusresults := updateStatus(ctx, agent, r.Client, status).WithError(err); updateStatusresults != nil {
			results = updateStatusresults
		}
	}()

	if !association.AreConfiguredIfSet(agent.GetAssociations(), r.recorder) {
		return results
	}

	// Run basic validations as a fallback in case webhook is disabled.
	if err = r.validate(ctx, agent); err != nil {
		results = results.WithError(err)
		return results
	}

	driverResults := internalReconcile(Params{
		Context:        ctx,
		Client:         r.Client,
		EventRecorder:  r.recorder,
		Watches:        r.dynamicWatches,
		Agent:          agent,
		OperatorParams: r.Parameters,
	}, &status)

	results = results.WithResults(driverResults)
	return results
}

func (r *ReconcileAgent) validate(ctx context.Context, agent agentv1alpha1.Agent) error {
	defer tracing.Span(&ctx)()

	// Run create validations only as update validations require old object which we don't have here.
	if err := agent.ValidateCreate(); err != nil {
		logconf.FromContext(ctx).Error(err, "Validation failed")
		k8s.EmitErrorEvent(r.recorder, err, &agent, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(ctx, err)
	}
	return nil
}

func (r *ReconcileAgent) onDelete(obj types.NamespacedName) {
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
}
