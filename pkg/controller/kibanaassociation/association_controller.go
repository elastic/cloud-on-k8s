// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"reflect"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	elasticsearchuser "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	kblabel "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
	// ElasticsearchCASecretSuffix is used as suffix for CAPublicCertSecretName
	ElasticsearchCASecretSuffix = "kb-es-ca" // nolint
)

var (
	log            = logf.Log.WithName(name)
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new Association Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := newReconciler(mgr, accessReviewer, params)
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileAssociation {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileAssociation{
		Client:         client,
		accessReviewer: accessReviewer,
		scheme:         mgr.GetScheme(),
		watches:        watches.NewDynamicWatches(),
		recorder:       mgr.GetEventRecorderFor(name),
		Parameters:     params,
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
	accessReviewer rbac.AccessReviewer
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileAssociation) onDelete(obj types.NamespacedName) error {
	// Clean up memory
	r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(obj))
	r.watches.Secrets.RemoveHandlerForKey(esCAWatchName(obj))
	// Delete user
	return user.DeleteUser(r.Client, NewUserLabelSelector(obj))
}

// Reconcile reads that state of the cluster for an Association object and makes changes based on the state read and what is in
// the Association.Spec
func (r *ReconcileAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "kibana_name", &r.iteration)()

	var kibana kbv1.Kibana
	if err := association.FetchWithAssociation(r.Client, request, &kibana); err != nil {
		if apierrors.IsNotFound(err) {
			// Kibana has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, err
	}

	// Kibana is being deleted, short-circuit reconciliation and remove artifacts related to the association.
	if !kibana.DeletionTimestamp.IsZero() {
		kbName := k8s.ExtractNamespacedName(&kibana)
		return reconcile.Result{}, r.onDelete(kbName)
	}

	if common.IsPaused(kibana.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
		return common.PauseRequeue, nil
	}

	compatible, err := r.isCompatible(&kibana)
	if err != nil || !compatible {
		return reconcile.Result{}, err
	}

	results := reconciler.Results{}
	newStatus, err := r.reconcileInternal(&kibana)
	if err != nil {
		results.WithError(err)
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

	return results.
		WithResult(rbac.NextReconciliation(r.accessReviewer)).
		WithResult(resultFromStatus(newStatus)).
		Aggregate()
}

func resultFromStatus(status commonv1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileAssociation) isCompatible(kibana *kbv1.Kibana) (bool, error) {
	selector := map[string]string{label.KibanaNameLabelName: kibana.Name}
	compat, err := annotation.ReconcileCompatibility(r.Client, kibana, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kibana, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}

	return compat, err
}

func (r *ReconcileAssociation) reconcileInternal(kibana *kbv1.Kibana) (commonv1.AssociationStatus, error) {
	kibanaKey := k8s.ExtractNamespacedName(kibana)

	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(r, kibana); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
	}

	if kibana.Spec.ElasticsearchRef.Name == "" {
		// stop watching any ES cluster previously referenced for this Kibana resource
		r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(kibanaKey))
		// other leftover resources are already garbage-collected
		return commonv1.AssociationUnknown, nil
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
		Watched: []types.NamespacedName{esRefKey},
		Watcher: kibanaKey,
	}); err != nil {
		return commonv1.AssociationFailed, err
	}

	userSecretKey := association.UserKey(kibana, kibanaUserSuffix)
	// watch the user secret in the ES namespace
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(kibanaKey),
		Watched: []types.NamespacedName{userSecretKey},
		Watcher: kibanaKey,
	}); err != nil {
		return commonv1.AssociationFailed, err
	}

	var es esv1.Elasticsearch
	if err := r.Get(esRefKey, &es); err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kibana, events.EventAssociationError, "Failed to find referenced backend %s: %v", esRefKey, err)
		if apierrors.IsNotFound(err) {
			// ES not found. 2 options:
			// - not created yet: that's ok, we'll reconcile on creation event
			// - deleted: existing resources will be garbage collected
			// in any case, since the user explicitly requested a managed association,
			// remove connection details if they are set
			if err := association.RemoveAssociationConf(r.Client, kibana); err != nil && !errors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch configuration from Kibana object",
					"namespace", kibana.Namespace, "kibana_name", kibana.Name)
				return commonv1.AssociationPending, err
			}

			return commonv1.AssociationPending, nil
		}
		return commonv1.AssociationFailed, err
	}

	// Check if reference to Elasticsearch is allowed to be established
	if allowed, err := association.IsAllowedReference(
		r.accessReviewer,
		kibana,
		&es,
		r,
	); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	if err := association.ReconcileEsUser(
		r.Client,
		r.scheme,
		kibana,
		map[string]string{
			AssociationLabelName:      kibana.Name,
			AssociationLabelNamespace: kibana.Namespace,
		},
		elasticsearchuser.KibanaSystemUserBuiltinRole,
		kibanaUserSuffix,
		es); err != nil {
		return commonv1.AssociationPending, err
	}

	caSecret, err := r.reconcileElasticsearchCA(kibana, esRefKey)
	if err != nil {
		return commonv1.AssociationPending, err
	}

	// construct the expected association configuration
	authSecret := association.ClearTextSecretKeySelector(kibana, kibanaUserSuffix)
	expectedESAssoc := &commonv1.AssociationConf{
		AuthSecretName: authSecret.Name,
		AuthSecretKey:  authSecret.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	// update the association configuration if necessary
	if !reflect.DeepEqual(expectedESAssoc, kibana.AssociationConf()) {
		log.Info("Updating Kibana spec with Elasticsearch backend configuration", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
		if err := association.UpdateAssociationConf(r.Client, kibana, expectedESAssoc); err != nil {
			if errors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update association configuration", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
			return commonv1.AssociationPending, err
		}
		kibana.SetAssociationConf(expectedESAssoc)
	}

	return commonv1.AssociationEstablished, nil
}

// Unbind remove the association resources
func (r *ReconcileAssociation) Unbind(kibana commonv1.Associated) error {
	kibanaKey := k8s.ExtractNamespacedName(kibana)
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := user.DeleteUser(r.Client, NewUserLabelSelector(kibanaKey)); err != nil {
		return err
	}
	// Also remove the association configuration
	return association.RemoveAssociationConf(r.Client, kibana)
}

func (r *ReconcileAssociation) reconcileElasticsearchCA(kibana *kbv1.Kibana, es types.NamespacedName) (association.CASecret, error) {
	kibanaKey := k8s.ExtractNamespacedName(kibana)
	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(kibanaKey),
		Watched: []types.NamespacedName{http.PublicCertsSecretRef(esv1.ESNamer, es)},
		Watcher: kibanaKey,
	}); err != nil {
		return association.CASecret{}, err
	}
	// Build the labels applied on the secret
	labels := kblabel.NewLabels(kibana.Name)
	labels[AssociationLabelName] = kibana.Name
	return association.ReconcileCASecret(
		r.Client,
		r.scheme,
		kibana,
		es,
		labels,
		ElasticsearchCASecretSuffix,
	)
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. Common use case is an Elasticsearch reference in Kibana spec that was removed.
func deleteOrphanedResources(c k8s.Client, kibana *kbv1.Kibana) error {
	var secrets corev1.SecretList
	ns := client.InNamespace(kibana.Namespace)
	matchLabels := NewResourceSelector(kibana.Name)
	if err := c.List(&secrets, ns, matchLabels); err != nil {
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
		if metav1.IsControlledBy(&s, kibana) || hasBeenCreatedBy(&s, kibana) {
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
