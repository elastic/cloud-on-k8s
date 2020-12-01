// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package agent

import (
	"context"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
	client := k8s.WrapClient(mgr.GetClient())
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

	// Watch dynamically referenced Secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAgent{}

// ReconcileAgent reconciles a Agent object
type ReconcileAgent struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a Agent object and makes changes based on the state read
// and what is in the Agent.Spec
func (r *ReconcileAgent) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	ctx := tracing.NewContextTransaction(r.Tracer, controllerName, request.String(), map[string]string{"iteration": string(r.iteration)})
	ctx = logconf.InitInContext(ctx, controllerName, r.iteration, request.Namespace, "agent_name", request.Name)

	defer common.LogReconciliationRun(logconf.FromContext(ctx), request, "agent_name", &r.iteration)()
	defer tracing.EndContextTransaction(ctx)

	// name -> name requested for
	// type -> controllerName
	// label-> iteration

	// transaction per request, passed with ctx
	// span per function?
	// logger params populated from ctx

	var agent agentv1alpha1.Agent
	if err := association.FetchWithAssociations(ctx, r.Client, request, &agent); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(request.NamespacedName)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(&agent) {
		logconf.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", agent.Namespace, "agent_name", agent.Name)
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(ctx, &agent); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if agent.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &agent, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	res, err := r.doReconcile(ctx, agent).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, &agent, events.EventReconciliationError, "Reconciliation error: %v", err)

	return res, err
}

func (r *ReconcileAgent) doReconcile(ctx context.Context, agent agentv1alpha1.Agent) *reconciler.Results {
	results := reconciler.NewResult(ctx)
	if !association.AreConfiguredIfSet(agent.GetAssociations(), r.recorder) {
		return results
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &agent); err != nil {
		return results.WithError(err)
	}

	driverResults := internalReconcile(NewParams(
		ctx,
		r.Client,
		r.recorder,
		r.dynamicWatches,
		agent,
	))
	results.WithResults(driverResults)

	return results
}

func (r *ReconcileAgent) validate(ctx context.Context, agent *agentv1alpha1.Agent) error {
	// todo
	//span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	//defer span.End()

	//if err := agent.ValidateCreate(); err != nil {//log.Error(err, "Validation failed")
	//k8s.EmitErrorEvent(r.recorder, err, agent, events.EventReasonValidation, err.Error())
	//return tracing.CaptureError(vctx, err)
	//}

	return nil
}

func (r *ReconcileAgent) isCompatible(ctx context.Context, agent *agentv1alpha1.Agent) (bool, error) {
	defer tracing.Span(ctx)()
	selector := map[string]string{NameLabelName: agent.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, agent, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, agent, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileAgent) onDelete(obj types.NamespacedName) {
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
}
