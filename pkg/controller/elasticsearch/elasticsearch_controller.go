// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"context"
	"sync/atomic"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/expectations"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	commonversion "github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/validation"
	esversion "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const name = "elasticsearch-controller"

var log = logf.Log.WithName(name)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
// this is also called by cmd/main.go
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, name, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileElasticsearch {
	client := k8s.WrapClient(mgr.GetClient())
	observerSettings := observer.DefaultSettings
	observerSettings.Tracer = params.Tracer
	return &ReconcileElasticsearch{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(name),
		licenseChecker: license.NewLicenseChecker(client, params.OperatorNamespace),
		esObservers:    observer.NewManager(observerSettings),

		dynamicWatches: watches.NewDynamicWatches(),
		expectations:   expectations.NewClustersExpectations(client),

		Parameters: params,
	}
}

func addWatches(c controller.Controller, r *ReconcileElasticsearch) error {
	// Watch for changes to Elasticsearch
	if err := c.Watch(
		&source.Kind{Type: &esv1.Elasticsearch{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	// Watch StatefulSets
	if err := c.Watch(
		&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &esv1.Elasticsearch{},
		},
	); err != nil {
		return err
	}

	// Watch pods belonging to ES clusters
	if err := watches.WatchPods(c, label.ClusterNameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &esv1.Elasticsearch{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}
	if err := r.dynamicWatches.Secrets.AddHandler(&watches.OwnerWatch{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &esv1.Elasticsearch{},
		},
	}); err != nil {
		return err
	}

	// Trigger a reconciliation when observers report a cluster health change
	if err := c.Watch(observer.WatchClusterHealthChange(r.esObservers), reconciler.GenericEventHandler()); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileElasticsearch{}

// ReconcileElasticsearch reconciles an Elasticsearch object
type ReconcileElasticsearch struct {
	k8s.Client
	operator.Parameters
	recorder       record.EventRecorder
	licenseChecker license.Checker

	esObservers *observer.Manager

	dynamicWatches watches.DynamicWatches

	// expectations help dealing with inconsistencies in our client cache,
	// by marking resources updates as expected, and skipping some operations if the cache is not up-to-date.
	expectations *expectations.ClustersExpectation

	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads the state of the cluster for an Elasticsearch object and makes changes based on the state read and
// what is in the Elasticsearch.Spec
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "es_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "elasticsearch")
	defer tracing.EndTransaction(tx)

	// Fetch the Elasticsearch instance
	var es esv1.Elasticsearch
	requeue, err := r.fetchElasticsearch(ctx, request, &es)
	if err != nil || requeue {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(&es) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{}, nil
	}

	selector := map[string]string{label.ClusterNameLabelName: es.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, &es, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, &es, events.EventCompatCheckError, "Error during compatibility check: %v", err)
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if !compat {
		// this resource is not able to be reconciled by this version of the controller, so we will skip it and not requeue
		return reconcile.Result{}, nil
	}

	// Remove any previous Finalizers
	if err := finalizer.RemoveAll(r.Client, &es); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	err = annotation.UpdateControllerVersion(ctx, r.Client, &es, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	state := esreconcile.NewState(es)
	results := r.internalReconcile(ctx, es, state)
	err = r.updateStatus(ctx, es, state)
	if err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status", "namespace", es.Namespace, "es_name", es.Name)
			return reconcile.Result{Requeue: true}, nil
		}
		k8s.EmitErrorEvent(r.recorder, err, &es, events.EventReconciliationError, "Reconciliation error: %v", err)
	}
	return results.WithError(err).Aggregate()
}

func (r *ReconcileElasticsearch) fetchElasticsearch(ctx context.Context, request reconcile.Request, es *esv1.Elasticsearch) (bool, error) {
	span, _ := apm.StartSpan(ctx, "fetch_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	err := r.Get(request.NamespacedName, es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// Additional cleanup is done by the onDelete function.
			r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
			return true, nil
		}
		// Error reading the object - requeue the request.
		return true, err
	}
	return false, nil
}

func (r *ReconcileElasticsearch) internalReconcile(
	ctx context.Context,
	es esv1.Elasticsearch,
	reconcileState *esreconcile.State,
) *reconciler.Results {
	results := reconciler.NewResult(ctx)

	if es.IsMarkedForDeletion() {
		// resource will be deleted, nothing to reconcile
		r.onDelete(k8s.ExtractNamespacedName(&es))
		return results
	}

	span, ctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	// this is the same validation as the webhook, but we run it again here in case the webhook has not been configured
	err := validation.ValidateElasticsearch(es)
	span.End()

	if err != nil {
		log.Error(
			err,
			"Elasticsearch manifest validation failed",
			"namespace", es.Namespace,
			"es_name", es.Name,
		)
		reconcileState.UpdateElasticsearchInvalid(err)
		return results
	}

	err = validation.CheckForWarnings(es)
	if err != nil {
		log.Info(
			"Elasticsearch manifest has warnings. Proceed at your own risk. "+err.Error(),
			"namespace", es.Namespace,
			"es_name", es.Name,
		)
		reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonValidation, err.Error())
	}

	ver, err := commonversion.Parse(es.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}
	supported := esversion.SupportedVersions(*ver)
	if supported == nil {
		return results.WithError(pkgerrors.Errorf("unsupported version: %s", ver))
	}

	return driver.NewDefaultDriver(driver.DefaultDriverParameters{
		OperatorParameters: r.Parameters,
		ES:                 es,
		ReconcileState:     reconcileState,
		Client:             r.Client,
		Recorder:           r.recorder,
		Version:            *ver,
		Expectations:       r.expectations.ForCluster(k8s.ExtractNamespacedName(&es)),
		Observers:          r.esObservers,
		DynamicWatches:     r.dynamicWatches,
		SupportedVersions:  *supported,
		LicenseChecker:     r.licenseChecker,
	}).Reconcile(ctx)
}

func (r *ReconcileElasticsearch) updateStatus(
	ctx context.Context,
	es esv1.Elasticsearch,
	reconcileState *esreconcile.State,
) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	events, cluster := reconcileState.Apply()
	for _, evt := range events {
		log.V(1).Info("Recording event", "event", evt)
		r.recorder.Event(&es, evt.EventType, evt.Reason, evt.Message)
	}
	if cluster == nil {
		return nil
	}
	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", es.Namespace,
		"es_name", es.Name,
		"status", cluster.Status,
	)
	return common.UpdateStatus(r.Client, cluster)
}

// onDelete garbage collect resources when a Elasticsearch cluster is deleted
func (r *ReconcileElasticsearch) onDelete(es types.NamespacedName) {
	r.expectations.RemoveCluster(es)
	r.esObservers.StopObserving(es)
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(es))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(esv1.ESNamer, es.Name))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(user.UserProvidedRolesWatchName(es))
	r.dynamicWatches.Secrets.RemoveHandlerForKey(user.UserProvidedFileRealmWatchName(es))
}
