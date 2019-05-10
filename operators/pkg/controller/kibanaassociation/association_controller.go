// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"reflect"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
)

// Kibana association controller
//
// This controller's only purpose is to complete a Kibana resource
// with connection details to an existing Elasticsearch cluster.
//
// High-level overview:
// - watch Kibana resources
// - if a Kibana resource specifies an Elasticsearch resource reference,
//   resolve details about that ES cluster (url, credentials), and update
//   the Kibana resource with ES connection details
// - create the Kibana user for Elasticsearch
// - copy ES CA public cert secret into Kibana namespace
// - reconcile on any change from watching Kibana, Elasticsearch, Users and secrets
//
// If reference to an Elasticsearch cluster is not set in the Kibana resource,
// this controller does nothing.

var (
	log            = logf.Log.WithName("kibana-association-controller")
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new Association Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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

var _ reconcile.Reconciler = &ReconcileAssociation{}

// ReconcileAssociation reconciles a Kibana resource for association with Elasticsearch
type ReconcileAssociation struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	watches  watches.DynamicWatches

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for an Association object and makes changes based on the state read and what is in
// the Association.Spec
func (r *ReconcileAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "request", request)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// retrieve Kibana resource
	var kibana kbtype.Kibana
	err := r.Get(request.NamespacedName, &kibana)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// register or execute watch finalizers
	h := finalizer.NewHandler(r)
	err = h.Handle(&kibana, watchFinalizer(k8s.ExtractNamespacedName(&kibana), r.watches))
	if err != nil {
		if apierrors.IsConflict(err) {
			log.Info("Conflict while handling finalizer")
			return reconcile.Result{Requeue: true}, nil
		}
		// failed to prepare or run finalizer: retry
		return defaultRequeue, err
	}

	// Kibana is being deleted: short-circuit reconciliation
	if !kibana.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	if common.IsPaused(kibana.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	newStatus, err := r.reconcileInternal(kibana)
	// maybe update status
	if !reflect.DeepEqual(kibana.Status.AssociationStatus, newStatus) {
		kibana.Status.AssociationStatus = newStatus
		if err := r.Status().Update(&kibana); err != nil {
			if apierrors.IsConflict(err) {
				log.Info("Conflict while updating status")
				return reconcile.Result{Requeue: true}, nil
			}

			return defaultRequeue, err
		}
	}
	return resultFromStatus(newStatus), err
}

func resultFromStatus(status commonv1alpha1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1alpha1.AssociationPending:
		return defaultRequeue // retry again
	case commonv1alpha1.AssociationEstablished, commonv1alpha1.AssociationFailed:
		return reconcile.Result{} // we are done or there is not much we can do
	default:
		return reconcile.Result{} // make the compiler happy
	}
}

func (r *ReconcileAssociation) reconcileInternal(kibana kbtype.Kibana) (commonv1alpha1.AssociationStatus, error) {
	kibanaKey := k8s.ExtractNamespacedName(&kibana)

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(r, kibana); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.")
	}

	if kibana.Spec.ElasticsearchRef.Name == "" {
		// stop watching any ES cluster previously referenced for this Kibana resource
		r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(kibanaKey))
		// other leftover resources are already garbage-collected
		return "", nil
	}

	// this Kibana instance references an Elasticsearch cluster
	esRef := kibana.Spec.ElasticsearchRef
	if esRef.Namespace == "" {
		// no namespace provided: default to Kibana's namespace
		esRef.Namespace = kibana.Namespace
	}
	esRefKey := esRef.NamespacedName()

	// watch the referenced ES cluster for future reconciliations
	if err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(kibanaKey),
		Watched: esRefKey,
		Watcher: kibanaKey,
	}); err != nil {
		return commonv1alpha1.AssociationFailed, err
	}

	var es estype.Elasticsearch
	if err := r.Get(esRefKey, &es); err != nil {
		if apierrors.IsNotFound(err) {
			// ES not found. 2 options:
			// - not created yet: that's ok, we'll reconcile on creation event
			// - deleted: existing resources will be garbage collected
			// in any case, since the user explicitly requested a managed association,
			// remove connection details if they are set
			if (kibana.Spec.Elasticsearch != kbtype.BackendElasticsearch{}) {
				kibana.Spec.Elasticsearch = kbtype.BackendElasticsearch{}
				log.Info("Removing Elasticsearch configuration from managed association", "kibana", kibana.Name)
				if err := r.Update(&kibana); err != nil {
					return commonv1alpha1.AssociationPending, err
				}
			}
			return commonv1alpha1.AssociationPending, nil
		}
		return commonv1alpha1.AssociationFailed, err
	}

	if err := reconcileEsUser(r.Client, r.scheme, kibana, esRefKey); err != nil {
		return commonv1alpha1.AssociationPending, err
	}

	caSecretName, err := r.reconcileCASecret(kibana, esRefKey)
	if err != nil {
		return commonv1alpha1.AssociationPending, err
	}

	// update Kibana resource with ES access details
	var expectedEsConfig kbtype.BackendElasticsearch
	expectedEsConfig.CaCertSecret = &caSecretName
	expectedEsConfig.URL = services.ExternalServiceURL(es)
	expectedEsConfig.Auth.SecretKeyRef = KibanaUserSecretSelector(kibana)

	if !reflect.DeepEqual(kibana.Spec.Elasticsearch, expectedEsConfig) {
		kibana.Spec.Elasticsearch = expectedEsConfig
		log.Info("Updating Kibana spec with Elasticsearch backend configuration")
		if err := r.Update(&kibana); err != nil {
			return commonv1alpha1.AssociationPending, err
		}
	}

	return commonv1alpha1.AssociationEstablished, nil
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. Common use case is an Elasticsearch reference in Kibana spec that was removed.
func deleteOrphanedResources(c k8s.Client, kibana kbtype.Kibana) error {
	hasESRef := kibana.Spec.ElasticsearchRef.Name != ""

	var secrets corev1.SecretList
	selector := NewResourceSelector(kibana.Name)
	if err := c.List(&client.ListOptions{LabelSelector: selector}, &secrets); err != nil {
		return err
	}
	for _, s := range secrets.Items {
		// look for association secrets owned by this kibana instance
		// which should not exist since no ES referenced in the spec
		if metav1.IsControlledBy(&s, &kibana) && !hasESRef {
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
	for _, u := range users.Items {
		// look for users owned by this Kibana instance
		// which should not exist since no ES referenced in the spec
		if metav1.IsControlledBy(&u, &kibana) && !hasESRef {
			log.Info("Deleting", "user", k8s.ExtractNamespacedName(&u))
			if err := c.Delete(&u); err != nil {
				return err
			}
		}
	}
	return nil
}
