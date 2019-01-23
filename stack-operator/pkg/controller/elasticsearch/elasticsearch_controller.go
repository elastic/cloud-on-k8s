package elasticsearch

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/finalizer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	commonversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/driver"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/observer"
	esreconcile "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshot"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	return addWatches(c, reconciler.dynamicWatches)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) (*ReconcileElasticsearch, error) {
	esCa, err := nodecerts.NewSelfSignedCa("elasticsearch-controller")
	if err != nil {
		return nil, err
	}

	return &ReconcileElasticsearch{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("elasticsearch-controller"),

		esCa:        esCa,
		esObservers: observer.NewManager(observer.DefaultSettings),

		finalizers:     finalizer.NewHandler(mgr.GetClient()),
		dynamicWatches: watches.NewDynamicWatches(),

		Parameters: params,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	return controller.New("elasticsearch-controller", mgr, controller.Options{Reconciler: r})
}

func addWatches(c controller.Controller, dynamicWatches watches.DynamicWatches) error {
	// Watch for changes to Elasticsearch
	if err := c.Watch(
		&source.Kind{Type: &elasticsearchv1alpha1.ElasticsearchCluster{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	// watch any pods created by Elasticsearch
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	}); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, dynamicWatches.Secrets); err != nil {
		return err
	}
	// Watch secrets
	if err := dynamicWatches.Secrets.AddHandler(&watches.OwnerWatch{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
		},
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileElasticsearch{}

// ReconcileElasticsearch reconciles a Elasticsearch object
type ReconcileElasticsearch struct {
	client.Client
	operator.Parameters
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	esCa *nodecerts.Ca

	esObservers *observer.Manager

	finalizers finalizer.Handler

	dynamicWatches watches.DynamicWatches

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and
// what is in the Elasticsearch.Spec
//
// Automatically generate RBAC rules:
// +kubebuilder:rbac:groups=,resources=pods;endpoints;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=elasticsearchclusters;elasticsearchclusters/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the Elasticsearch instance
	es := elasticsearchv1alpha1.ElasticsearchCluster{}
	err := r.Get(context.TODO(), request.NamespacedName, &es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	state := esreconcile.NewState(es)
	results := r.internalReconcile(es, state)
	err = r.updateStatus(es, state)
	return results.WithError(err).Aggregate()
}

func (r *ReconcileElasticsearch) internalReconcile(
	es elasticsearchv1alpha1.ElasticsearchCluster,
	reconcileState *esreconcile.State,
) *esreconcile.Results {
	results := &esreconcile.Results{}

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

	driver, err := driver.NewDriver(driver.Options{
		Client: r.Client,
		Scheme: r.scheme,

		Version: *ver,

		ClusterCa:      r.esCa,
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
	es elasticsearchv1alpha1.ElasticsearchCluster,
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
	return r.Status().Update(context.TODO(), cluster)
}

// finalizersFor returns the list of finalizers applying to a given es cluster
func (r *ReconcileElasticsearch) finalizersFor(
	es elasticsearchv1alpha1.ElasticsearchCluster,
	watched watches.DynamicWatches,
) []finalizer.Finalizer {
	clusterName := k8s.ExtractNamespacedName(es.ObjectMeta)
	return []finalizer.Finalizer{
		r.esObservers.Finalizer(clusterName),
		snapshot.Finalizer(clusterName, watched),
	}
}
