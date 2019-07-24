// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package elasticsearch

import (
	"sync/atomic"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	elasticsearchv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	commonversion "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/driver"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/validation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

const name = "elasticsearch-controller"

var log = logf.Log.WithName(name)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields
// on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileElasticsearch {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileElasticsearch{
		Client:   client,
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder(name),

		esObservers: observer.NewManager(observer.DefaultSettings),

		finalizers:     finalizer.NewHandler(client),
		dynamicWatches: watches.NewDynamicWatches(),
		expectations:   driver.NewGenerationExpectations(),

		Parameters: params,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	return controller.New(name, mgr, controller.Options{Reconciler: r})
}

func addWatches(c controller.Controller, r *ReconcileElasticsearch) error {
	// Watch for changes to Elasticsearch
	if err := c.Watch(
		&source.Kind{Type: &elasticsearchv1alpha1.Elasticsearch{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	// Watch StatefulSets
	if err := c.Watch(
		&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &elasticsearchv1alpha1.Elasticsearch{},
		},
	); err != nil {
		return err
	}

	// Watch pods belonging to ES clusters
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(
				func(object handler.MapObject) []reconcile.Request {
					labels := object.Meta.GetLabels()
					clusterName, isSet := labels[label.ClusterNameLabelName]
					if !isSet {
						return nil
					}
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Namespace: object.Meta.GetNamespace(),
								Name:      clusterName,
							},
						},
					}
				}),
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
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	esObservers *observer.Manager

	finalizers finalizer.Handler

	dynamicWatches watches.DynamicWatches

	// expectations help dealing with inconsistencies in our client cache,
	// by marking resources updates as expected, and skipping some operations if the cache is not up-to-date.
	expectations *driver.Expectations

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads the state of the cluster for an Elasticsearch object and makes changes based on the state read and
// what is in the Elasticsearch.Spec
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "namespace", request.Namespace, "es_name", request.Name)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime), "namespace", request.Namespace, "es_name", request.Name)
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
		log.Info("Object is paused. Skipping reconciliation", "namespace", es.Namespace, "es_name", es.Name, "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	selector := labels.Set(map[string]string{label.ClusterNameLabelName: es.Name}).AsSelector()
	compat, err := annotation.ReconcileCompatibility(r.Client, &es, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !compat {
		// this resource is not able to be reconciled by this version of the controller, so we will skip it and not requeue
		return reconcile.Result{}, nil
	}

	err = annotation.UpdateControllerVersion(r.Client, &es, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, err
	}

	state := esreconcile.NewState(es)
	results := r.internalReconcile(es, state)
	err = r.updateStatus(es, state)
	if err != nil && apierrors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status", "namespace", es.Namespace, "es_name", es.Name)
		return reconcile.Result{Requeue: true}, nil
	}
	return results.WithError(err).Aggregate()
}

func (r *ReconcileElasticsearch) internalReconcile(
	es elasticsearchv1alpha1.Elasticsearch,
	reconcileState *esreconcile.State,
) *reconciler.Results {
	results := &reconciler.Results{}

	if err := r.finalizers.Handle(&es, r.finalizersFor(es)...); err != nil {
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
		Client:   r.Client,
		Scheme:   r.scheme,
		Recorder: r.recorder,

		Version: *ver,

		Expectations:   r.expectations,
		Observers:      r.esObservers,
		DynamicWatches: r.dynamicWatches,
		Parameters:     r.Parameters,
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
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration), "namespace", es.Namespace, "es_name", es.Name)
	events, cluster := reconcileState.Apply()
	for _, evt := range events {
		log.V(1).Info("Recording event", "event", evt)
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
) []finalizer.Finalizer {
	clusterName := k8s.ExtractNamespacedName(&es)
	return []finalizer.Finalizer{
		r.esObservers.Finalizer(clusterName),
		keystore.Finalizer(k8s.ExtractNamespacedName(&es), r.dynamicWatches, es.Kind()),
		http.DynamicWatchesFinalizer(r.dynamicWatches, es.Name, esname.ESNamer),
	}
}
