// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"reflect"
	"sync/atomic"
	"time"

	apmtype "github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	associationsv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/association"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/finalizer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	log            = logf.Log.WithName("apm-es-association-controller")
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new ApmServerElasticsearchAssociation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
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
func newReconciler(mgr manager.Manager) (*ReconcileApmServerElasticsearchAssociation, error) {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileApmServerElasticsearchAssociation{
		Client:   client,
		scheme:   mgr.GetScheme(),
		watches:  watches.NewDynamicWatches(),
		recorder: mgr.GetRecorder("association-controller"),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New("apm-es-association-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func addWatches(c controller.Controller, r *ReconcileApmServerElasticsearchAssociation) error {
	// Watch for changes to the association
	if err := c.Watch(&source.Kind{Type: &associationsv1alpha1.ApmServerElasticsearchAssociation{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch Elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &estype.Elasticsearch{}}, r.watches.ElasticsearchClusters); err != nil {
		return err
	}

	// Watch ApmServer objects
	if err := c.Watch(&source.Kind{Type: &apmtype.ApmServer{}}, r.watches.ApmServers); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileApmServerElasticsearchAssociation{}

// ReconcileApmServerElasticsearchAssociation reconciles a ApmServerElasticsearchAssociation object
type ReconcileApmServerElasticsearchAssociation struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	watches  watches.DynamicWatches

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a ApmServerElasticsearchAssociation object and makes changes based on the state read
// and what is in the ApmServerElasticsearchAssociation.Spec
func (r *ReconcileApmServerElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	var association associationsv1alpha1.ApmServerElasticsearchAssociation
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
	err = handler.Handle(&association, watchFinalizer(k8s.ExtractNamespacedName(&association), r.watches))
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

func elasticsearchWatchName(assocKey types.NamespacedName) string {
	return assocKey.Namespace + "-" + assocKey.Name + "-es-watch"
}

func apmServerWatchName(assocKey types.NamespacedName) string {
	return assocKey.Namespace + "-" + assocKey.Name + "-apm-server-watch"
}

// watchFinalizer ensure that we remove watches for Apm Servers and Elasticsearch clusters that we are no longer interested in
// because the assocation has been deleted.
func watchFinalizer(assocKey types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches",
		Execute: func() error {
			w.ApmServers.RemoveHandlerForKey(apmServerWatchName(assocKey))
			w.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(assocKey))
			return nil
		},
	}
}

func resultFromStatus(status associationsv1alpha1.AssociationStatus) reconcile.Result {
	switch status {
	case associationsv1alpha1.AssociationPending:
		return defaultRequeue // retry again
	case associationsv1alpha1.AssociationEstablished, associationsv1alpha1.AssociationFailed:
		return reconcile.Result{} // we are done or there is not much we can do
	default:
		return reconcile.Result{} // make the compiler happy
	}
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileInternal(association associationsv1alpha1.ApmServerElasticsearchAssociation) (associationsv1alpha1.AssociationStatus, error) {
	assocKey := k8s.ExtractNamespacedName(&association)

	// Make sure we see events from ApmServer+Elasticsearch using a dynamic watch
	// will become more relevant once we refactor user handling to CRDs and implement
	// syncing of user credentials across namespaces
	err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(assocKey),
		Watched: association.Spec.Elasticsearch.NamespacedName(),
		Watcher: assocKey,
	})
	if err != nil {
		return associationsv1alpha1.AssociationFailed, err
	}
	err = r.watches.ApmServers.AddHandler(watches.NamedWatch{
		Name:    apmServerWatchName(assocKey),
		Watched: association.Spec.ApmServer.NamespacedName(),
		Watcher: assocKey,
	})
	if err != nil {
		return associationsv1alpha1.AssociationFailed, err
	}

	var es estype.Elasticsearch
	err = r.Get(association.Spec.Elasticsearch.NamespacedName(), &es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Es not found, could be deleted or not yet created? Recheck in a while
			return associationsv1alpha1.AssociationPending, nil
		}
		return associationsv1alpha1.AssociationFailed, err
	}

	// TODO reconcile external user CRD here
	err = reconcileEsUser(r.Client, r.scheme, association)
	if err != nil {
		return associationsv1alpha1.AssociationPending, err // TODO distinguish conflicts and non-recoverable errors here
	}

	var expectedEsConfig apmtype.ElasticsearchOutput

	// TODO: look up CA name from the ES cluster resource
	var publicCACertSecret corev1.Secret
	publicCACertSecretKey := types.NamespacedName{Namespace: es.Namespace, Name: nodecerts.CASecretNameForCluster(es.Name)}
	if err = r.Get(publicCACertSecretKey, &publicCACertSecret); err != nil {
		return associationsv1alpha1.AssociationPending, err // maybe not created yet
	}
	// TODO this is currently limiting the association to the same namespace
	expectedEsConfig.SSL.CertificateAuthoritiesSecret = &publicCACertSecret.Name
	expectedEsConfig.Hosts = []string{services.ExternalServiceURL(es)}
	expectedEsConfig.Auth.SecretKeyRef = clearTextSecretKeySelector(association)

	var currentApmServer apmtype.ApmServer
	if err := r.Get(association.Spec.ApmServer.NamespacedName(), &currentApmServer); err != nil {
		if apierrors.IsNotFound(err) {
			return associationsv1alpha1.AssociationPending, err
		}
		return associationsv1alpha1.AssociationFailed, err
	}

	// TODO: this is a bit rough
	if !reflect.DeepEqual(currentApmServer.Spec.Output.Elasticsearch, expectedEsConfig) {
		currentApmServer.Spec.Output.Elasticsearch = expectedEsConfig
		log.Info("Updating Apm Server spec with Elasticsearch output configuration")
		if err := r.Update(&currentApmServer); err != nil {
			return associationsv1alpha1.AssociationPending, err
		}
	}

	if err := deleteOrphanedResources(r, association); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	return associationsv1alpha1.AssociationEstablished, nil
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association the standard reconcile mechanism will not delete the
// now redundant old user object/secret. This function lists all resources that don't match the current name/namespace
// combinations and deletes them.
func deleteOrphanedResources(c k8s.Client, assoc associationsv1alpha1.ApmServerElasticsearchAssociation) error {
	var secrets corev1.SecretList
	selector := association.NewResourceSelector(assoc.Name)
	if err := c.List(&client.ListOptions{LabelSelector: selector}, &secrets); err != nil {
		return err
	}
	expectedSecretKey := secretKey(assoc)
	for _, s := range secrets.Items {
		if k8s.ExtractNamespacedName(&s) != expectedSecretKey {
			log.Info("Deleting", "secret", k8s.ExtractNamespacedName(&s))
			if err := c.Delete(&s); err != nil {
				return err
			}
		}
	}

	var users estype.UserList
	if err := c.List(&client.ListOptions{LabelSelector: selector}, &users); err != nil {
		return err
	}
	expectedUserKey := userKey(assoc)
	for _, u := range users.Items {
		if k8s.ExtractNamespacedName(&u) != expectedUserKey {
			log.Info("Deleting", "user", k8s.ExtractNamespacedName(&u))
			if err := c.Delete(&u); err != nil {
				return err
			}
		}
	}
	return nil
}
