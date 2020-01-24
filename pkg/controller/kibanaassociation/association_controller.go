// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibanaassociation

import (
	"context"
	"reflect"
	"time"

	commonapm "github.com/elastic/cloud-on-k8s/pkg/controller/common/apm"
	"go.elastic.co/apm"
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
		recorder:   mgr.GetEventRecorderFor(name),
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
	defer common.LogReconciliationRun(log, request, &r.iteration)()
	tx, ctx := commonapm.NewTransaction(r.Tracer, request.NamespacedName, "kibana_association")
	defer commonapm.EndTransaction(tx)

	var kibana kbv1.Kibana
	if err := association.FetchWithAssociation(ctx, r.Client, request, &kibana); err != nil {
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

	compatible, err := r.isCompatible(ctx, &kibana)
	if err != nil || !compatible {
		return reconcile.Result{}, err
	}

	newStatus, err := r.reconcileInternal(ctx, &kibana)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, &kibana, events.EventReconciliationError, "Reconciliation error: %v", err)
	}

	// maybe update status
	result, err := r.updateStatus(ctx, kibana, newStatus)
	if err != nil || reflect.DeepEqual(result, reconcile.Result{}) {
		return result, err
	}
	return resultFromStatus(newStatus), err
}

func (r *ReconcileAssociation) updateStatus(ctx context.Context, kibana kbv1.Kibana, newStatus commonv1.AssociationStatus) (reconcile.Result, error) {
	span, _ := apm.StartSpan(ctx, "update_status", "app")
	defer span.End()
	if !reflect.DeepEqual(kibana.Status.AssociationStatus, newStatus) {
		oldStatus := kibana.Status.AssociationStatus
		kibana.Status.AssociationStatus = newStatus
		if err := r.Status().Update(&kibana); err != nil {
			if apierrors.IsConflict(err) {
				// Conflicts are expected and will be resolved on next loop
				log.V(1).Info("Conflict while updating status", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
				return reconcile.Result{Requeue: true}, nil
			}

			return defaultRequeue, commonapm.CaptureError(ctx, err)
		}
		r.recorder.AnnotatedEventf(&kibana,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)
	}
	return reconcile.Result{}, nil
}

func resultFromStatus(status commonv1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileAssociation) isCompatible(ctx context.Context, kibana *kbv1.Kibana) (bool, error) {
	selector := map[string]string{label.KibanaNameLabelName: kibana.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, kibana, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kibana, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}

	return compat, err
}

func (r *ReconcileAssociation) reconcileInternal(ctx context.Context, kibana *kbv1.Kibana) (commonv1.AssociationStatus, error) {
	kibanaKey := k8s.ExtractNamespacedName(kibana)
	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r, kibana); err != nil {
		log.Error(commonapm.CaptureError(ctx, err), "Error while trying to delete orphaned resources. Continuing.", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
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

	es, status, err := r.getElasticsearch(ctx, kibana, esRefKey)
	if status != "" || err != nil {
		return status, err
	}

	if err := association.ReconcileEsUser(
		ctx,
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
		return commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
	}

	caSecret, err := r.reconcileElasticsearchCA(ctx, kibana, esRefKey)
	if err != nil {
		return commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
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
	return r.updateAssociationConf(ctx, expectedESAssoc, kibana)
}

func (r *ReconcileAssociation) updateAssociationConf(ctx context.Context, expectedESAssoc *commonv1.AssociationConf, kibana *kbv1.Kibana) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_assoc_conf", "app")
	defer span.End()
	if !reflect.DeepEqual(expectedESAssoc, kibana.AssociationConf()) {
		log.Info("Updating Kibana spec with Elasticsearch backend configuration", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
		if err := association.UpdateAssociationConf(r.Client, kibana, expectedESAssoc); err != nil {
			if errors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update association configuration", "namespace", kibana.Namespace, "kibana_name", kibana.Name)
			return commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
		}
		kibana.SetAssociationConf(expectedESAssoc)
	}
	return commonv1.AssociationEstablished, nil
}

func (r *ReconcileAssociation) getElasticsearch(ctx context.Context, kibana *kbv1.Kibana, esRefKey types.NamespacedName) (esv1.Elasticsearch, commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "get_elasticsearch", "app")
	defer span.End()
	var es esv1.Elasticsearch
	if err := r.Get(esRefKey, &es); err != nil {
		k8s.EmitErrorEvent(r.recorder, err, kibana, events.EventAssociationError, "Failed to find referenced backend %s: %v", esRefKey, err)
		if apierrors.IsNotFound(err) {
			// ES not found. 2 options:
			// - not created yet: that's ok, we'll reconcile on creation event
			// - deleted: existing resources will be garbage collected
			// in any case, since the user explicitly requested a managed association,
			// remove connection details if they are set
			span, ctx = apm.StartSpan(ctx, "remove_assoc_conf", "app")
			if err := association.RemoveAssociationConf(r.Client, kibana); err != nil && !errors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch configuration from Kibana object",
					"namespace", kibana.Namespace, "kibana_name", kibana.Name)
				return es, commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
			}
			span.End()

			return es, commonv1.AssociationPending, nil
		}
		return es, commonv1.AssociationFailed, commonapm.CaptureError(ctx, err)
	}
	return es, "", nil
}

func (r *ReconcileAssociation) reconcileElasticsearchCA(ctx context.Context, kibana *kbv1.Kibana, es types.NamespacedName) (association.CASecret, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_es_ca", "app")
	defer span.End()
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
func deleteOrphanedResources(ctx context.Context, c k8s.Client, kibana *kbv1.Kibana) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources", "app")
	defer span.End()

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
