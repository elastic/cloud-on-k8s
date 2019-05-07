// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"fmt"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	elasticsearchv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	commonversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("elasticsearch-controller")
)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler, err := newReconciler(mgr, params)
	if err != nil {
		return err
	}
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) (*ReconcileElasticsearch, error) {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileElasticsearch{
		Client:   client,
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("elasticsearch-controller"),

		csrClient:   certificates.NewCertInitializerCSRClient(params.Dialer, certificates.CSRRequestTimeout),
		esObservers: observer.NewManager(params.Dialer, client, observer.DefaultSettings),

		finalizers:       finalizer.NewHandler(client),
		dynamicWatches:   watches.NewDynamicWatches(),
		podsExpectations: reconciler.NewExpectations(),

		Parameters: params,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	return controller.New("elasticsearch-controller", mgr, controller.Options{Reconciler: r})
}

func addWatches(c controller.Controller, r *ReconcileElasticsearch) error {
	// Watch for changes to Elasticsearch
	if err := c.Watch(
		&source.Kind{Type: &elasticsearchv1alpha1.Elasticsearch{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	// Watch pods
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, r.dynamicWatches.Pods); err != nil {
		return err
	}
	if err := r.dynamicWatches.Pods.AddHandlers(
		// trigger reconciliation loop on ES pods owned by this controller
		&watches.OwnerWatch{
			EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &elasticsearchv1alpha1.Elasticsearch{},
			},
		},
		// Reconcile pods expectations.
		// This does not technically need to be part of a dynamic watch, since it will
		// stay there forever (nothing dynamic here).
		// Turns out our dynamic watch mechanism happens to be a pretty nice way to
		// setup multiple "static" handlers for a single watch.
		watches.NewExpectationsWatch(
			"pods-expectations",
			r.podsExpectations,
			// retrieve cluster name from pod labels
			label.ClusterFromResourceLabels,
		)); err != nil {
		return err
	}

	// watch trust relationships and queue reconciliation for their associated cluster on changes
	if err := c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.TrustRelationship{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: label.NewToRequestsFuncFromClusterNameLabel(),
	}); err != nil {
		return err
	}

	// Watch remote clusters and queue reconciliation for their associated cluster on changes.
	if err := c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.RemoteCluster{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: label.NewToRequestsFuncFromClusterNameLabel(),
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.Elasticsearch{},
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
			OwnerType:    &elasticsearchv1alpha1.Elasticsearch{},
		},
	}); err != nil {
		return err
	}

	// ClusterLicense
	if err := c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.ClusterLicense{}}, r.dynamicWatches.ClusterLicense); err != nil {
		return err
	}

	// Users
	if err := c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.User{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: label.NewToRequestsFuncFromClusterNameLabel(),
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

// ReconcileElasticsearch reconciles a Elasticsearch object
type ReconcileElasticsearch struct {
	k8s.Client
	operator.Parameters
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	csrClient certificates.CSRClient

	esObservers *observer.Manager

	finalizers finalizer.Handler

	dynamicWatches watches.DynamicWatches

	// podsExpectations help dealing with inconsistencies in our client cache,
	// by marking Pods creation/deletion as expected, and waiting til they are effectively observed.
	podsExpectations *reconciler.Expectations

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and
// what is in the Elasticsearch.Spec
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "request", request)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime), "request", request)
	}()

	// Fetch the Elasticsearch instance
	es := elasticsearchv1alpha1.Elasticsearch{}
	err := r.Get(request.NamespacedName, &es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if common.IsPaused(es.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	state := esreconcile.NewState(es)
	results := r.internalReconcile(es, state)
	err = r.updateStatus(es, state)
	return results.WithError(err).Aggregate()
}

func (r *ReconcileElasticsearch) internalReconcile(
	es elasticsearchv1alpha1.Elasticsearch,
	reconcileState *esreconcile.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	if err := r.finalizers.Handle(&es, r.finalizersFor(es, r.dynamicWatches)...); err != nil {
		return results.WithError(err)
	}

	if es.IsMarkedForDeletion() {
		// resource will be deleted, nothing to reconcile
		// pre-delete operations are handled by finalizers
		return results
	}

	ver, err := commonversion.Parse(es.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}

	violations, err := validation.Validate(es)
	if err != nil {
		return results.WithError(err)
	}
	if len(violations) > 0 {
		reconcileState.UpdateElasticsearchInvalid(violations)
		return results
	}

	driver, err := driver.NewDriver(driver.Options{
		Client: r.Client,
		Scheme: r.scheme,

		Version: *ver,

		CSRClient:        r.csrClient,
		Observers:        r.esObservers,
		DynamicWatches:   r.dynamicWatches,
		PodsExpectations: r.podsExpectations,
		Parameters:       r.Parameters,
	})
	if err != nil {
		return results.WithError(err)
	}

	return driver.Reconcile(es, reconcileState)
}

func (r *ReconcileElasticsearch) updateStatus(
	es elasticsearchv1alpha1.Elasticsearch,
	reconcileState *esreconcile.State,
) error {
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	events, cluster := reconcileState.Apply()
	for _, evt := range events {
		log.Info(fmt.Sprintf("Recording event %+v", evt))
		r.recorder.Event(&es, evt.EventType, evt.Reason, evt.Message)
	}
	if cluster == nil {
		return nil
	}
	return r.Status().Update(cluster)
}

// finalizersFor returns the list of finalizers applying to a given es cluster
func (r *ReconcileElasticsearch) finalizersFor(
	es elasticsearchv1alpha1.Elasticsearch,
	watched watches.DynamicWatches,
) []finalizer.Finalizer {
	clusterName := k8s.ExtractNamespacedName(&es)
	return []finalizer.Finalizer{
		reconciler.ExpectationsFinalizer(clusterName, r.podsExpectations),
		r.esObservers.Finalizer(clusterName),
		settings.SecureSettingsFinalizer(clusterName, watched),
		license.Finalizer(clusterName, watched),
	}
}
