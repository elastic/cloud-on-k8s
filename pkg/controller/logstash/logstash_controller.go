// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

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

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/pipelines"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/validation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName = "logstash-controller"
)

// Add creates a new Logstash Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, r, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r)
}

// newReconciler returns a new reconcile.Reconciler.
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileLogstash {
	client := mgr.GetClient()
	return &ReconcileLogstash{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
		expectations:   expectations.NewClustersExpectations(client),
	}
}

// addWatches adds watches for all resources this controller cares about
func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileLogstash) error {
	// Watch for changes to Logstash
	if err := c.Watch(source.Kind(mgr.GetCache(), &logstashv1alpha1.Logstash{}, &handler.TypedEnqueueRequestForObject[*logstashv1alpha1.Logstash]{})); err != nil {
		return err
	}

	// Watch StatefulSets
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &appsv1.StatefulSet{}, handler.TypedEnqueueRequestForOwner[*appsv1.StatefulSet](
			mgr.GetScheme(), mgr.GetRESTMapper(),
			&logstashv1alpha1.Logstash{}, handler.OnlyControllerOwner(),
		))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` is correctly reconciled on any change.
	// Watching StatefulSets only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, labels.NameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&logstashv1alpha1.Logstash{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&logstashv1alpha1.Logstash{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, logstashv1alpha1.Kind); err != nil {
		return err
	}

	// Watch dynamically referenced Secrets
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcileLogstash{}

// ReconcileLogstash reconciles a Logstash object
type ReconcileLogstash struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration    uint64
	expectations *expectations.ClustersExpectation
}

// Reconcile reads that state of the cluster for a Logstash object and makes changes based on the state read
// and what is in the Logstash.Spec
// Automatically generate RBAC rules to allow the Controller to read and write StatefulSets
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=logstash.k8s.elastic.co,resources=logstashes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=logstash.k8s.elastic.co,resources=logstashes/status,verbs=get;update;patch
func (r *ReconcileLogstash) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "logstash_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	logstash := &logstashv1alpha1.Logstash{}
	if err := r.Client.Get(ctx, request.NamespacedName, logstash); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx, request.NamespacedName)
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, logstash) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation")
		return reconcile.Result{}, nil
	}

	if logstash.IsMarkedForDeletion() {
		return reconcile.Result{}, nil
	}

	results, status := r.doReconcile(ctx, *logstash)
	logger := ulog.FromContext(ctx)

	err := updateStatus(ctx, *logstash, r.Client, status)
	if err != nil {
		if apierrors.IsConflict(err) {
			logger.V(1).Info("Conflict while updating status. Requeueing", "namespace", logstash.Namespace, "ls_name", logstash.Name)
			return reconcile.Result{Requeue: true}, nil
		}
		k8s.MaybeEmitErrorEvent(r.recorder, err, logstash, events.EventReconciliationError, "Reconciliation error: %v", err)
	}
	return results.WithError(err).Aggregate()
}

func (r *ReconcileLogstash) doReconcile(ctx context.Context, logstash logstashv1alpha1.Logstash) (*reconciler.Results, logstashv1alpha1.LogstashStatus) {
	defer tracing.Span(&ctx)()
	logger := ulog.FromContext(ctx)
	results := reconciler.NewResult(ctx)
	status := newStatus(logstash)

	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, logstash.GetAssociations(), r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !areAssocsConfigured {
		return results, status
	}

	logstashVersion, err := version.Parse(logstash.Spec.Version)
	if err != nil {
		return results.WithError(err), status
	}
	assocAllowed, err := association.AllowVersion(logstashVersion, &logstash, logger, r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !assocAllowed {
		return results, status // will eventually retry
	}

	// Run basic validations as a fallback in case webhook is disabled.
	if err := r.validate(ctx, logstash); err != nil {
		results = results.WithError(err)
		return results, status
	}

	return internalReconcile(Params{
		Context:        ctx,
		Client:         r.Client,
		EventRecorder:  r.recorder,
		Watches:        r.dynamicWatches,
		Logstash:       logstash,
		Status:         status,
		OperatorParams: r.Parameters,
		Expectations:   r.expectations.ForCluster(k8s.ExtractNamespacedName(&logstash)),
	})
}

func (r *ReconcileLogstash) validate(ctx context.Context, logstash logstashv1alpha1.Logstash) error {
	defer tracing.Span(&ctx)()

	// Run create validations only as update validations require old object which we don't have here.
	if err := validation.ValidateLogstash(&logstash); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, &logstash, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(ctx, err)
	}
	return nil
}

func (r *ReconcileLogstash) onDelete(ctx context.Context, obj types.NamespacedName) error {
	r.expectations.RemoveCluster(obj)
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(pipelines.RefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, logstashv1alpha1.Kind)
}
