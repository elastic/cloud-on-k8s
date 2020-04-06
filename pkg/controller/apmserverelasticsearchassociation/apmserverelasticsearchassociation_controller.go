// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"context"
	"reflect"
	"strings"
	"time"

	"go.elastic.co/apm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/user"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/pkg/utils/rbac"
)

const (
	name                        = "apm-es-association-controller"
	apmUserSuffix               = "apm-user"
	elasticsearchCASecretSuffix = "apm-es-ca" // nolint
)

var (
	log            = logf.Log.WithName(name)
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// getRoles returns for a given version of the APM Server the set of required roles.
func getRoles(v version.Version) string {
	// 7.5.x and above
	if v.IsSameOrAfter(version.From(7, 5, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV75, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"ingest_admin",      // Set up index templates
			"apm_system",        // To collect metrics about APM Server
		}, ",")
	}

	// 7.1.x to 7.4.x
	if v.IsSameOrAfter(version.From(7, 1, 0)) {
		return strings.Join([]string{
			user.ApmUserRoleV7, // Retrieve cluster details (e.g. version) and manage apm-* indices
			"ingest_admin",     // Set up index templates
			"apm_system",       // To collect metrics about APM Server
		}, ",")
	}

	// 6.8
	return strings.Join([]string{
		user.ApmUserRoleV6, // Retrieve cluster details (e.g. version) and manage apm-* indices
		"apm_system",       // To collect metrics about APM Server
	}, ",")
}

// Add creates a new ApmServerElasticsearchAssociation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) error {
	r := newReconciler(mgr, accessReviewer, params)
	c, err := common.NewController(mgr, name, r, params)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, accessReviewer rbac.AccessReviewer, params operator.Parameters) *ReconcileApmServerElasticsearchAssociation {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileApmServerElasticsearchAssociation{
		Client:         client,
		accessReviewer: accessReviewer,
		watches:        watches.NewDynamicWatches(),
		recorder:       mgr.GetEventRecorderFor(name),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileApmServerElasticsearchAssociation) error {
	// Watch for changes to ApmServers
	if err := c.Watch(&source.Kind{Type: &apmv1.ApmServer{}}, &handler.EnqueueRequestForObject{}); err != nil {
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

	// Watch Secrets owned by an ApmServer resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &apmv1.ApmServer{},
		IsController: true,
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileApmServerElasticsearchAssociation{}

// ReconcileApmServerElasticsearchAssociation reconciles a ApmServerElasticsearchAssociation object
type ReconcileApmServerElasticsearchAssociation struct {
	k8s.Client
	accessReviewer rbac.AccessReviewer
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileApmServerElasticsearchAssociation) onDelete(obj types.NamespacedName) error {
	// Remove watcher on the Elasticsearch cluster
	r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(obj))
	// Remove watcher on the Elasticsearch CA secret
	r.watches.Secrets.RemoveHandlerForKey(esCAWatchName(obj))
	// Remove watcher on the user Secret in the Elasticsearch namespace
	r.watches.Secrets.RemoveHandlerForKey(elasticsearchWatchName(obj))
	// Delete user Secret in the Elasticsearch namespace
	return k8s.DeleteSecretMatching(r.Client, newUserLabelSelector(obj))
}

// Reconcile reads that state of the cluster for a ApmServerElasticsearchAssociation object and makes changes based on the state read
// and what is in the ApmServerElasticsearchAssociation.Spec
func (r *ReconcileApmServerElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "as_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "apm-es-association")
	defer tracing.EndTransaction(tx)

	var apmServer apmv1.ApmServer
	if err := association.FetchWithAssociation(ctx, r.Client, request, &apmServer); err != nil {
		if apierrors.IsNotFound(err) {
			// APM Server has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(apmServer.ObjectMeta) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
		return reconcile.Result{}, nil
	}

	// ApmServer is being deleted, short-circuit reconciliation and remove artifacts related to the association.
	if !apmServer.DeletionTimestamp.IsZero() {
		apmName := k8s.ExtractNamespacedName(&apmServer)
		return reconcile.Result{}, tracing.CaptureError(ctx, r.onDelete(apmName))
	}

	if compatible, err := r.isCompatible(ctx, &apmServer); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &apmServer, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	results := reconciler.NewResult(ctx)
	newStatus, err := r.reconcileInternal(ctx, &apmServer)
	if err != nil {
		results.WithError(err)
	}

	// we want to attempt a status update even in the presence of errors
	if err := r.updateStatus(ctx, apmServer, newStatus); err != nil {
		return defaultRequeue, tracing.CaptureError(ctx, err)
	}
	return results.
		WithError(err).
		WithResult(association.RequeueRbacCheck(r.accessReviewer)).
		WithResult(resultFromStatus(newStatus)).
		Aggregate()
}

func (r *ReconcileApmServerElasticsearchAssociation) updateStatus(ctx context.Context, apmServer apmv1.ApmServer, newStatus commonv1.AssociationStatus) error {
	span, _ := apm.StartSpan(ctx, "update_association", tracing.SpanTypeApp)
	defer span.End()

	oldStatus := apmServer.Status.Association
	if !reflect.DeepEqual(oldStatus, newStatus) {
		apmServer.Status.Association = newStatus
		if err := r.Status().Update(&apmServer); err != nil {
			return err
		}
		r.recorder.AnnotatedEventf(&apmServer,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)

	}
	return nil
}

func elasticsearchWatchName(assocKey types.NamespacedName) string {
	return assocKey.Namespace + "-" + assocKey.Name + "-es-watch"
}

// esCAWatchName returns the name of the watch setup on the secret that
// contains the HTTP certificate chain of Elasticsearch.
func esCAWatchName(apm types.NamespacedName) string {
	return apm.Namespace + "-" + apm.Name + "-ca-watch"
}

func resultFromStatus(status commonv1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileApmServerElasticsearchAssociation) isCompatible(ctx context.Context, apmServer *apmv1.ApmServer) (bool, error) {
	selector := map[string]string{labels.ApmServerNameLabelName: apmServer.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, apmServer, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileInternal(ctx context.Context, apmServer *apmv1.ApmServer) (commonv1.AssociationStatus, error) {
	// garbage collect leftover resources that are not required anymore
	if err := deleteOrphanedResources(ctx, r, apmServer); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
	}
	apmServerKey := k8s.ExtractNamespacedName(apmServer)
	// no auto-association nothing to do
	elasticsearchRef := apmServer.Spec.ElasticsearchRef
	if !elasticsearchRef.IsDefined() {
		// clean up watchers and remove artifacts related to the association
		if err := r.onDelete(apmServerKey); err != nil {
			return commonv1.AssociationFailed, err
		}
		// remove the configuration in the annotation, other leftover resources are already garbage-collected
		return commonv1.AssociationUnknown, association.RemoveAssociationConf(r.Client, apmServer)
	}
	if elasticsearchRef.Namespace == "" {
		// no namespace provided: default to the APM server namespace
		elasticsearchRef.Namespace = apmServer.Namespace
	}

	// Make sure we see events from Elasticsearch using a dynamic watch
	err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(apmServerKey),
		Watched: []types.NamespacedName{elasticsearchRef.NamespacedName()},
		Watcher: apmServerKey,
	})
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	userSecretKey := association.UserKey(apmServer, apmUserSuffix)
	// watch the user secret in the ES namespace
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(apmServerKey),
		Watched: []types.NamespacedName{userSecretKey},
		Watcher: apmServerKey,
	}); err != nil {
		return commonv1.AssociationFailed, err
	}

	var es esv1.Elasticsearch
	associationStatus, err := r.getElasticsearch(ctx, apmServer, elasticsearchRef, &es)
	if associationStatus != "" || err != nil {
		return associationStatus, err
	}

	// Check if reference to Elasticsearch is allowed to be established
	if allowed, err := association.CheckAndUnbind(
		r.accessReviewer,
		apmServer,
		&es,
		r,
		r.recorder,
	); err != nil || !allowed {
		return commonv1.AssociationPending, err
	}

	if err := association.ReconcileEsUser(
		ctx,
		r.Client,
		apmServer,
		associationLabels(apmServer),
		getRoles(version.MustParse(apmServer.Spec.Version)),
		apmUserSuffix,
		es,
	); err != nil { // TODO distinguish conflicts and non-recoverable errors here
		return commonv1.AssociationPending, err
	}

	caSecret, err := r.reconcileElasticsearchCA(ctx, apmServer, elasticsearchRef.NamespacedName())
	if err != nil {
		return commonv1.AssociationPending, err // maybe not created yet
	}

	// construct the expected ES output configuration
	authSecretRef := association.ClearTextSecretKeySelector(apmServer, apmUserSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	var status commonv1.AssociationStatus
	status, err = r.updateAssocConf(ctx, expectedAssocConf, apmServer)
	if err != nil || status != "" {
		return status, err
	}

	return commonv1.AssociationEstablished, nil
}

func (r *ReconcileApmServerElasticsearchAssociation) getElasticsearch(ctx context.Context, apmServer *apmv1.ApmServer, elasticsearchRef commonv1.ObjectSelector, es *esv1.Elasticsearch) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "get_elasticsearch", tracing.SpanTypeApp)
	defer span.End()

	err := r.Get(elasticsearchRef.NamespacedName(), es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := association.RemoveAssociationConf(r.Client, apmServer); err != nil && !apierrors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch output from APMServer object", "namespace", apmServer.Namespace, "name", apmServer.Name)
				return commonv1.AssociationPending, err
			}

			return commonv1.AssociationPending, nil
		}
		return commonv1.AssociationFailed, err
	}
	return "", nil
}

func (r *ReconcileApmServerElasticsearchAssociation) updateAssocConf(ctx context.Context, expectedAssocConf *commonv1.AssociationConf, apmServer *apmv1.ApmServer) (commonv1.AssociationStatus, error) {
	span, _ := apm.StartSpan(ctx, "update_apm_assoc", tracing.SpanTypeApp)
	defer span.End()

	if !reflect.DeepEqual(expectedAssocConf, apmServer.AssociationConf()) {
		log.Info("Updating APMServer spec with Elasticsearch association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
		if err := association.UpdateAssociationConf(r.Client, apmServer, expectedAssocConf); err != nil {
			if apierrors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update APMServer association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
			return commonv1.AssociationPending, err
		}
		apmServer.SetAssociationConf(expectedAssocConf)
	}
	return "", nil
}

// Unbind removes the association resources
func (r *ReconcileApmServerElasticsearchAssociation) Unbind(apm commonv1.Associated) error {
	apmKey := k8s.ExtractNamespacedName(apm)
	// Ensure that user in Elasticsearch is deleted to prevent illegitimate access
	if err := k8s.DeleteSecretMatching(r.Client, newUserLabelSelector(apmKey)); err != nil {
		return err
	}
	// Also remove the association configuration
	return association.RemoveAssociationConf(r.Client, apm)
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileElasticsearchCA(ctx context.Context, as *apmv1.ApmServer, es types.NamespacedName) (association.CASecret, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_es_ca", tracing.SpanTypeApp)
	defer span.End()

	apmKey := k8s.ExtractNamespacedName(as)
	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(apmKey),
		Watched: []types.NamespacedName{certificates.PublicCertsSecretRef(esv1.ESNamer, es)},
		Watcher: apmKey,
	}); err != nil {
		return association.CASecret{}, err
	}

	return association.ReconcileCASecret(
		r.Client,
		as,
		es,
		maps.Merge(labels.NewLabels(as.Name), associationLabels(as)),
		elasticsearchCASecretSuffix,
	)
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association the standard reconcile mechanism will not delete the
// now redundant old user object/secret. This function lists all resources that don't match the current name/namespace
// combinations and deletes them.
func deleteOrphanedResources(ctx context.Context, c k8s.Client, as *apmv1.ApmServer) error {
	return association.DeleteOrphanedResources(ctx, c, as, associationLabels(as))
}
