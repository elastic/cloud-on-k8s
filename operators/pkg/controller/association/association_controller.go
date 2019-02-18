// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package association

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	associations "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	v1alpha12 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log            = logf.Log.WithName("association-controller")
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this deployments.Add(mgr) to install this Controller
func Add(mgr manager.Manager, _ operator.Parameters) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (*ReconcileAssociation, error) {

	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileAssociation{
		Client:   client,
		scheme:   mgr.GetScheme(),
		watches:  watches.NewDynamicWatches(),
		recorder: mgr.GetRecorder("association-controller"),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New("association-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return c, err
	}
	return c, nil
}

func addWatches(c controller.Controller, r *ReconcileAssociation) error {

	// Watch for changes to the Stack
	if err := c.Watch(&source.Kind{Type: &associations.KibanaElasticsearchAssociation{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &v1alpha1.ElasticsearchCluster{}}, r.watches.Clusters); err != nil {
		return err
	}

	// Watch kibana objects
	if err := c.Watch(&source.Kind{Type: &v1alpha12.Kibana{}}, r.watches.Kibanas); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileAssociation{}

// ReconcileAssociation reconciles a Kibana-Elasticsearch association object
type ReconcileAssociation struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	watches  watches.DynamicWatches

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and what is in
// the Elasticsearch.Spec
func (r *ReconcileAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	var association associations.KibanaElasticsearchAssociation
	err := r.Get(request.NamespacedName, &association)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if common.IsPaused(association.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	handler := finalizer.NewHandler(r)
	err = handler.Handle(&association, watchFinalizer(association.Name, r.watches))
	if err != nil {
		// failed to prepare finalizer or run finalizer: retry
		return defaultRequeue, err
	}

	// Association is being deleted short-circuit reconciliation
	if !association.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	newStatus, err := r.reconcileInternal(association)
	// maybe update status
	origStatus := association.Status.DeepCopy()
	association.Status.AssociationStatus = newStatus

	if !reflect.DeepEqual(*origStatus, association.Status) {
		if err := r.Status().Update(&association); err != nil {
			return defaultRequeue, err
		}
	}
	return resultFromStatus(newStatus), err

}

func elasticsearchWatchName(assocName string) string {
	return assocName + "-es-watch"
}

func kibanaWatchName(assocName string) string {
	return assocName + "-kb-watch"
}

func watchFinalizer(assocName string, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches",
		Execute: func() error {
			w.Kibanas.RemoveHandlerForKey(kibanaWatchName(assocName))
			w.Clusters.RemoveHandlerForKey(elasticsearchWatchName(assocName))
			return nil
		},
	}
}

func resultFromStatus(status associations.AssociationStatus) reconcile.Result {
	switch status {
	case associations.AssociationPending:
		return defaultRequeue // retry again
	case associations.AssociationEstablished, associations.AssociationFailed:
		return reconcile.Result{} // we are done or there is not much we can do
	default:
		return reconcile.Result{} // make the compiler happy
	}
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and what is in
// the Elasticsearch.Spec
func (r *ReconcileAssociation) reconcileInternal(association associations.KibanaElasticsearchAssociation) (associations.AssociationStatus, error) {

	assocKey := k8s.ExtractNamespacedName(&association)

	// Make sure we see events from Kibana+Elasticsearch using a dynamic watch
	// will become more relevant once we refactor user handling to CRDs and implement
	// syncing of user credentials across namespaces
	err := r.watches.Clusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(association.Name),
		Watched: association.Spec.Elasticsearch.NamespacedName(),
		Watcher: assocKey,
	})
	if err != nil {
		return associations.AssociationFailed, err
	}
	err = r.watches.Kibanas.AddHandler(watches.NamedWatch{
		Name:    kibanaWatchName(association.Name),
		Watched: association.Spec.Kibana.NamespacedName(),
		Watcher: assocKey,
	})
	if err != nil {
		return associations.AssociationFailed, err
	}

	var es v1alpha1.ElasticsearchCluster
	err = r.Get(association.Spec.Elasticsearch.NamespacedName(), &es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Es not found, could be deleted or not yet created? Recheck in a while
			return associations.AssociationPending, nil
		}
		return associations.AssociationFailed, err
	}

	// TODO reconcile external user CRD here

	var kb v1alpha12.BackendElasticsearch
	// TODO: be dynamic wrt to the service name
	kb.URL = fmt.Sprintf("https://%s:9200", services.ExternalServiceName(es.Name))

	internalUsersSecretName := secret.ElasticInternalUsersSecretName(es.Name)
	var internalUsersSecret corev1.Secret
	internalUsersSecretKey := types.NamespacedName{Namespace: es.Namespace, Name: internalUsersSecretName}
	if err := r.Get(internalUsersSecretKey, &internalUsersSecret); err != nil {
		return associations.AssociationPending, err
	}

	// TODO: can deliver through a shared secret instead?
	kb.Auth.Inline = &v1alpha12.ElasticsearchInlineAuth{
		Username: secret.InternalKibanaServerUserName,
		// TODO: error checking
		Password: string(internalUsersSecret.Data[secret.InternalKibanaServerUserName]),
	}

	var publicCACertSecret corev1.Secret
	publicCACertSecretKey := types.NamespacedName{Namespace: es.Namespace, Name: es.Name}
	if err = r.Get(publicCACertSecretKey, &publicCACertSecret); err != nil {
		return associations.AssociationPending, err // maybe not created yet
	}
	kb.CaCertSecret = &publicCACertSecret.Name

	var currentKb v1alpha12.Kibana
	if err := r.Get(association.Spec.Kibana.NamespacedName(), &currentKb); err != nil && !apierrors.IsNotFound(err) {
		return associations.AssociationPending, err
	}

	// TODO: this is a bit rough
	if !reflect.DeepEqual(currentKb.Spec.Elasticsearch, kb) {
		currentKb.Spec.Elasticsearch = kb
		log.Info("Updating Kibana spec")
		if err := r.Update(&currentKb); err != nil {
			return associations.AssociationPending, err
		}
	}

	return associations.AssociationEstablished, nil
}
