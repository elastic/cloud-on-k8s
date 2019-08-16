// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"reflect"
	"sync/atomic"
	"time"

	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	elasticsearchuser "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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

const (
	name = "kibana-association-controller"
	// kibanaUserSuffix is used to suffix user and associated secret resources.
	kibanaUserSuffix = "kibana-user"
)

var (
	log            = logf.Log.WithName(name)
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new Association Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	r := newReconciler(mgr, params)
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileAssociation {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileAssociation{
		Client:     client,
		scheme:     mgr.GetScheme(),
		watches:    watches.NewDynamicWatches(),
		recorder:   mgr.GetRecorder(name),
		Parameters: params,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
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
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for an Association object and makes changes based on the state read and what is in
// the Association.Spec
func (r *ReconcileAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "namespace", request.Namespace, "kibana_name", request.Name)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime), "namespace", request.Namespace, "kibana_name", request.Name)
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
	err = h.Handle(
		&kibana,
		watchFinalizer(k8s.ExtractNamespacedName(&kibana), r.watches),
		user.UserFinalizer(r.Client, NewUserLabelSelector(k8s.ExtractNamespacedName(&kibana))),
	)
	if err != nil {
		if apierrors.IsConflict(err) {
			// Conflicts are expected here and should be resolved on next loop
			log.V(1).Info("Conflict while handling finalizer")
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
		log.Info("Object is paused. Skipping reconciliation", "namespace", kibana.Namespace, "kibana_name", kibana.Name, "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	selector := labels.Set(map[string]string{label.KibanaNameLabelName: kibana.Name}).AsSelector()
	compat, err := annotation.ReconcileCompatibility(r.Client, &kibana, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, &kibana, events.EventCompatCheckError, "Error during compatibility check: %v", err)
		return reconcile.Result{}, err
	}
	if !compat {
		// this resource is not able to be reconciled by this version of the controller, so we will skip it and not requeue
		return reconcile.Result{}, nil
	}

	newStatus, err := r.reconcileInternal(kibana)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, &kibana, events.EventReconciliationError, "Reconciliation error: %v", err)
	}

	// maybe update status
	if !reflect.DeepEqual(kibana.Status.AssociationStatus, newStatus) {
		oldStatus := kibana.Status.AssociationStatus
		kibana.Status.AssociationStatus = newStatus
		if err := r.Status().Update(&kibana); err != nil {
			if apierrors.IsConflict(err) {
				// Conflicts are expected and will be resolved on next loop
				log.V(1).Info("Conflict while updating status", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
				return reconcile.Result{Requeue: true}, nil
			}

			return defaultRequeue, err
		}
		r.recorder.AnnotatedEventf(&kibana,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)
	}
	return resultFromStatus(newStatus), err
}

func resultFromStatus(status commonv1alpha1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1alpha1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileAssociation) reconcileInternal(kibana kbtype.Kibana) (commonv1alpha1.AssociationStatus, error) {
	kibanaKey := k8s.ExtractNamespacedName(&kibana)

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(r, kibana); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
	}

	if kibana.Spec.ElasticsearchRef.Name == "" {
		// stop watching any ES cluster previously referenced for this Kibana resource
		r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(kibanaKey))
		// other leftover resources are already garbage-collected
		return commonv1alpha1.AssociationUnknown, nil
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

	userSecretKey := association.UserKey(&kibana, kibanaUserSuffix)
	// watch the user secret in the ES namespace
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(kibanaKey),
		Watched: userSecretKey,
		Watcher: kibanaKey,
	}); err != nil {
		return commonv1alpha1.AssociationFailed, err
	}

	var es estype.Elasticsearch
	if err := r.Get(esRefKey, &es); err != nil {
		k8s.EmitErrorEvent(r.recorder, err, &kibana, events.EventAssociationError, "Failed to find referenced backend %s: %v", esRefKey, err)
		if apierrors.IsNotFound(err) {
			// ES not found. 2 options:
			// - not created yet: that's ok, we'll reconcile on creation event
			// - deleted: existing resources will be garbage collected
			// in any case, since the user explicitly requested a managed association,
			// remove connection details if they are set
			if (kibana.Spec.Elasticsearch != kbtype.BackendElasticsearch{}) {
				kibana.Spec.Elasticsearch = kbtype.BackendElasticsearch{}
				log.Info("Removing Elasticsearch configuration from managed association", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
				if err := r.Update(&kibana); err != nil {
					return commonv1alpha1.AssociationPending, err
				}
			}
			return commonv1alpha1.AssociationPending, nil
		}
		return commonv1alpha1.AssociationFailed, err
	}

	if err := association.ReconcileEsUser(
		r.Client,
		r.scheme,
		&kibana,
		map[string]string{
			AssociationLabelName:      kibana.Name,
			AssociationLabelNamespace: kibana.Namespace,
		},
		elasticsearchuser.KibanaSystemUserBuiltinRole,
		kibanaUserSuffix,
		es); err != nil {
		return commonv1alpha1.AssociationPending, err
	}

	caSecretName, err := r.reconcileCASecret(kibana, esRefKey)
	if err != nil {
		return commonv1alpha1.AssociationPending, err
	}

	// update Kibana resource with ES access details
	var expectedEsConfig kbtype.BackendElasticsearch
	expectedEsConfig.CertificateAuthorities.SecretName = caSecretName
	expectedEsConfig.URL = services.ExternalServiceURL(es)
	expectedEsConfig.Auth.SecretKeyRef = association.ClearTextSecretKeySelector(&kibana, kibanaUserSuffix)

	if !reflect.DeepEqual(kibana.Spec.Elasticsearch, expectedEsConfig) {
		kibana.Spec.Elasticsearch = expectedEsConfig
		log.Info("Updating Kibana spec with Elasticsearch backend configuration", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
		if err := r.Update(&kibana); err != nil {
			return commonv1alpha1.AssociationPending, err
		}
	}

	return commonv1alpha1.AssociationEstablished, nil
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. Common use case is an Elasticsearch reference in Kibana spec that was removed.
func deleteOrphanedResources(c k8s.Client, kibana kbtype.Kibana) error {
	var secrets corev1.SecretList
	selector := NewResourceSelector(kibana.Name)
	if err := c.List(&client.ListOptions{LabelSelector: selector, Namespace: kibana.Namespace}, &secrets); err != nil {
		return err
	}

	// Namespace in reference can be empty, in that case we compare it with the namespace of Kibana
	var esRefNamespace string
	if kibana.Spec.ElasticsearchRef.IsDefined() && kibana.Spec.ElasticsearchRef.Namespace != "" {
		esRefNamespace = kibana.Spec.ElasticsearchRef.Namespace
	} else {
		esRefNamespace = kibana.Namespace
	}

	for _, s := range secrets.Items {
		if metav1.IsControlledBy(&s, &kibana) || hasBeenCreatedBy(&s, kibana) {
			if !kibana.Spec.ElasticsearchRef.IsDefined() {
				// look for association secrets owned by this kibana instance
				// which should not exist since no ES referenced in the spec
				log.Info("Deleting secret", "namespace", s.Namespace, "secret_name", s.Name, "kibana_name", kibana.Name)
				if err := c.Delete(&s); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			} else if value, ok := s.Labels[common.TypeLabelName]; ok && value == user.UserType &&
				esRefNamespace != s.Namespace {
				// User secret may live in an other namespace, check if it has changed
				log.Info("Deleting secret", "namespace", s.Namespace, "secretname", s.Name, "kibana_name", kibana.Name)
				if err := c.Delete(&s); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
		}
	}
	return nil
}
