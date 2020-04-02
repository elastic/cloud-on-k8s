// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package entsearchassociation

import (
	"context"
	"reflect"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

// TODO: this is almost exactly the same code as the apm server association controller
//  -> massive refactoring in common association stuff should happen.

const (
	name                        = "entsearch-es-association-controller"
	entSearchUserSuffix         = "entsearch-es-user"
	elasticsearchCASecretSuffix = "entsearch-es-ca" // nolint
)

var (
	log            = logf.Log.WithName(name)
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := newReconciler(mgr, accessReviewer, params)
	c, err := common.NewController(mgr, name, r, params)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

func newReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileEnterpriseSearchElasticsearchAssociation {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileEnterpriseSearchElasticsearchAssociation{
		Client:         client,
		accessReviewer: accessReviewer,
		scheme:         mgr.GetScheme(),
		watches:        watches.NewDynamicWatches(),
		recorder:       mgr.GetEventRecorderFor(name),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileEnterpriseSearchElasticsearchAssociation) error {
	// Watch for changes to EnterpriseSearch
	if err := c.Watch(&source.Kind{Type: &entsv1beta1.EnterpriseSearch{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch Elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &esv1.Elasticsearch{}}, r.watches.ElasticsearchClusters); err != nil {
		return err
	}

	// Dynamically watch Elasticsearch public CA secrets for referenced ES clusters
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.watches.Secrets); err != nil {
		return err
	}

	// Watch Secrets owned by an EnterpriseSearch resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &entsv1beta1.EnterpriseSearch{},
		IsController: true,
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileEnterpriseSearchElasticsearchAssociation{}

// ReconcileEnterpriseSearchElasticsearchAssociation reconciles the association between
// EnterpriseSearch and Elasticsearch
type ReconcileEnterpriseSearchElasticsearchAssociation struct {
	k8s.Client
	accessReviewer rbac.AccessReviewer
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) onDelete(obj types.NamespacedName) error {
	// Clean up memory
	r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(obj))
	r.watches.Secrets.RemoveHandlerForKey(esCAWatchName(obj))
	// Delete user
	return k8s.DeleteSecretMatching(r.Client, NewUserLabelSelector(obj))
}

// Reconcile reads that state of the cluster for a ReconcileEnterpriseSearchElasticsearchAssociation object
// and makes changes based on the state read and what is in the ReconcileEnterpriseSearchElasticsearchAssociation.Spec
func (r *ReconcileEnterpriseSearchElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "ents_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "entsearch-es-association")
	defer tracing.EndTransaction(tx)

	var entSearch entsv1beta1.EnterpriseSearch
	if err := association.FetchWithAssociation(ctx, r.Client, request, &entSearch); err != nil {
		if apierrors.IsNotFound(err) {
			// EnterpriseSearch resource has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(entSearch.ObjectMeta) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", entSearch.Namespace, "ents_name", entSearch.Name)
		return reconcile.Result{}, nil
	}

	// EnterpriseSearch is being deleted, short-circuit reconciliation and remove artifacts related to the association.
	if !entSearch.DeletionTimestamp.IsZero() {
		entSearchName := k8s.ExtractNamespacedName(&entSearch)
		return reconcile.Result{}, tracing.CaptureError(ctx, r.onDelete(entSearchName))
	}

	if compatible, err := r.isCompatible(ctx, &entSearch); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &entSearch, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := reconciler.NewResult(ctx)
	newStatus, err := r.reconcileInternal(ctx, &entSearch)
	if err != nil {
		results.WithError(err)
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, entSearch, newStatus); err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithError(err).
		WithResult(association.RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatus(newStatus)).
		Aggregate()
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) isCompatible(ctx context.Context, entSearch *entsv1beta1.EnterpriseSearch) (bool, error) {
	selector := map[string]string{enterprisesearch.EnterpriseSearchNameLabelName: entSearch.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, entSearch, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, entSearch, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func elasticsearchWatchName(assocKey types.NamespacedName) string {
	return assocKey.Namespace + "-" + assocKey.Name + "-es-watch"
}

// esCAWatchName returns the name of the watch setup on the secret that
// contains the HTTP certificate chain of Elasticsearch.
func esCAWatchName(entsearch types.NamespacedName) string {
	return entsearch.Namespace + "-" + entsearch.Name + "-ca-watch"
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) reconcileInternal(ctx context.Context, entSearch *entsv1beta1.EnterpriseSearch) (commonv1.AssociationStatus, error) {
	// no auto-association nothing to do
	elasticsearchRef := entSearch.Spec.ElasticsearchRef
	if !elasticsearchRef.IsDefined() {
		return commonv1.AssociationUnknown, nil
	}
	if elasticsearchRef.Namespace == "" {
		// no namespace provided: default to the Enterprise Search namespace
		elasticsearchRef.Namespace = entSearch.Namespace
	}
	assocKey := k8s.ExtractNamespacedName(entSearch)
	// Make sure we see events from Elasticsearch using a dynamic watch
	err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(assocKey),
		Watched: []types.NamespacedName{elasticsearchRef.NamespacedName()},
		Watcher: assocKey,
	})
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	var es esv1.Elasticsearch
	associationStatus, err := r.getElasticsearch(ctx, entSearch, elasticsearchRef, &es)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// Check if reference to Elasticsearch is allowed to be established
	if allowed, err := association.CheckAndUnbind(
		r.accessReviewer,
		entSearch,
		&es,
		r,
		r.recorder,
	); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	if err := association.ReconcileEsUser(
		ctx,
		r.Client,
		entSearch,
		map[string]string{
			AssociationLabelName:      entSearch.Name,
			AssociationLabelNamespace: entSearch.Namespace,
		},
		"superuser",
		entSearchUserSuffix,
		es,
	); err != nil { // TODO distinguish conflicts and non-recoverable errors here
		return commonv1.AssociationPending, err
	}

	caSecret, err := r.reconcileElasticsearchCA(ctx, entSearch, elasticsearchRef.NamespacedName())
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	// construct the expected ES output configuration
	authSecretRef := association.ClearTextSecretKeySelector(entSearch, entSearchUserSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	var status commonv1.AssociationStatus
	status, err = r.updateAssocConf(ctx, expectedAssocConf, entSearch)
	if err != nil || status != "" {
		return status, err
	}

	if err := deleteOrphanedResources(ctx, r, entSearch); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", entSearch.Namespace, "ents_name", entSearch.Name)
	}
	return commonv1.AssociationEstablished, nil
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) updateStatus(ctx context.Context, entSearch entsv1beta1.EnterpriseSearch, newStatus commonv1.AssociationStatus) error {
	span, _ := apm.StartSpan(ctx, "update_association", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := entSearch.Status.Association
	if !reflect.DeepEqual(oldStatus, newStatus) {
		entSearch.Status.Association = newStatus
		if err := r.Status().Update(&entSearch); err != nil {
			return err
		}
		r.recorder.AnnotatedEventf(&entSearch,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)

	}
	return nil
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) getElasticsearch(ctx context.Context, entSearch *entsv1beta1.EnterpriseSearch, elasticsearchRef commonv1.ObjectSelector, es *esv1.Elasticsearch) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	err := r.Get(elasticsearchRef.NamespacedName(), es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, entSearch, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := association.RemoveAssociationConf(r.Client, entSearch); err != nil && !apierrors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch output from EnterpriseSearch object", "namespace", entSearch.Namespace, "name", entSearch.Name)
				return commonv1.AssociationPending, err
			}
			return commonv1.AssociationPending, nil
		}
		return commonv1.AssociationFailed, err
	}
	return "", nil
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association the standard reconcile mechanism will not delete the
// now redundant old user object/secret. This function lists all resources that don't match the current name/namespace
// combinations and deletes them.
func deleteOrphanedResources(ctx context.Context, c k8s.Client, entSearch *entsv1beta1.EnterpriseSearch) error {
	span, _ := apm.StartSpan(ctx, "delete_orphaned_resources", tracing.SpanTypeApp)
	defer span.End()

	var secrets corev1.SecretList
	ns := client.InNamespace(entSearch.Namespace)
	matchLabels := client.MatchingLabels(NewResourceLabels(entSearch.Name))
	if err := c.List(&secrets, ns, matchLabels); err != nil {
		return err
	}

	for _, s := range secrets.Items {
		controlledBy := metav1.IsControlledBy(&s, entSearch)
		if controlledBy && !entSearch.Spec.ElasticsearchRef.IsDefined() {
			log.Info("Deleting secret", "namespace", s.Namespace, "secret_name", s.Name, "ents_name", entSearch.Name)
			if err := c.Delete(&s); err != nil {
				return err
			}
		}
	}
	return nil
}

func resultFromStatus(status commonv1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) reconcileElasticsearchCA(ctx context.Context, entSearch *entsv1beta1.EnterpriseSearch, es types.NamespacedName) (association.CASecret, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_es_ca", tracing.SpanTypeApp)
	defer span.End()

	entSearchKey := k8s.ExtractNamespacedName(entSearch)
	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(entSearchKey),
		Watched: []types.NamespacedName{certificates.PublicCertsSecretRef(esv1.ESNamer, es)},
		Watcher: entSearchKey,
	}); err != nil {
		return association.CASecret{}, err
	}
	// Build the labels applied on the secret
	labels := enterprisesearch.Labels(entSearch.Name)
	labels[AssociationLabelName] = entSearch.Name
	return association.ReconcileCASecret(
		r.Client,
		entSearch,
		es,
		labels,
		elasticsearchCASecretSuffix,
	)
}

func (r *ReconcileEnterpriseSearchElasticsearchAssociation) updateAssocConf(ctx context.Context, expectedAssocConf *commonv1.AssociationConf, entSearch *entsv1beta1.EnterpriseSearch) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_entsearch_assoc", tracing.SpanTypeApp)
	defer span.End()

	if !reflect.DeepEqual(expectedAssocConf, entSearch.AssociationConf()) {
		log.Info("Updating EnterpriseSearch spec with Elasticsearch association configuration", "namespace", entSearch.Namespace, "name", entSearch.Name)
		if err := association.UpdateAssociationConf(r.Client, entSearch, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update EnterpriseSearch association configuration", "namespace", entSearch.Namespace, "name", entSearch.Name)
			return commonv1.AssociationPending, err
		}
		entSearch.SetAssociationConf(expectedAssocConf)
	}
	return "", nil
}

// Unbind removes the association resources
func (r *ReconcileEnterpriseSearchElasticsearchAssociation) Unbind(entSearch commonv1.Associated) error {
	entSearchKey := k8s.ExtractNamespacedName(entSearch)
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := k8s.DeleteSecretMatching(r.Client, NewUserLabelSelector(entSearchKey)); err != nil {
		return err
	}
	// Also remove the association configuration
	return association.RemoveAssociationConf(r.Client, entSearch)
}
