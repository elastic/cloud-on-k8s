// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

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
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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

// Add creates a new ApmServerElasticsearchAssociation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileApmServerElasticsearchAssociation {
	client := k8s.WrapClient(mgr.GetClient())
	var tracer *apm.Tracer
	if params.EnableAPM {
		tracer = commonapm.NewTracer("apmserver_assoc_controller", log)
	}
	return &ReconcileApmServerElasticsearchAssociation{
		Client:     client,
		scheme:     mgr.GetScheme(),
		watches:    watches.NewDynamicWatches(),
		recorder:   mgr.GetEventRecorderFor(name),
		Parameters: params,
		tracer:     tracer,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return nil, err
	}
	return c, nil
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
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	watches  watches.DynamicWatches
	operator.Parameters
	tracer *apm.Tracer
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileApmServerElasticsearchAssociation) onDelete(obj types.NamespacedName) error {
	// Clean up memory
	r.watches.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(obj))
	r.watches.Secrets.RemoveHandlerForKey(esCAWatchName(obj))
	// Delete user
	return user.DeleteUser(r.Client, NewUserLabelSelector(obj))
}

// Reconcile reads that state of the cluster for a ApmServerElasticsearchAssociation object and makes changes based on the state read
// and what is in the ApmServerElasticsearchAssociation.Spec
func (r *ReconcileApmServerElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, &r.iteration)()
	tx, ctx := commonapm.NewTransaction(r.tracer, request.NamespacedName, "apmserver_assoc")
	defer commonapm.EndTransaction(tx)

	var span *apm.Span
	span, ctx = apm.StartSpan(ctx, "fetch_assoc", "app")
	var apmServer apmv1.ApmServer
	if err := association.FetchWithAssociation(r.Client, request, &apmServer); err != nil {
		if apierrors.IsNotFound(err) {
			// APM Server has been deleted, remove artifacts related to the association.
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, commonapm.CaptureError(ctx, err)
	}
	span.End()

	if common.IsPaused(apmServer.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
		return common.PauseRequeue, nil
	}

	// ApmServer is being deleted, short-circuit reconciliation and remove artifacts related to the association.
	if !apmServer.DeletionTimestamp.IsZero() {
		apmName := k8s.ExtractNamespacedName(&apmServer)
		return reconcile.Result{}, commonapm.CaptureError(ctx, r.onDelete(apmName))
	}

	if compatible, err := r.isCompatible(&apmServer); err != nil || !compatible {
		return reconcile.Result{}, err
	}

	span, ctx = apm.StartSpan(ctx, "update_controller_version", "app")
	if err := annotation.UpdateControllerVersion(r.Client, &apmServer, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, commonapm.CaptureError(ctx, err)
	}
	span.End()

	newStatus, err := r.reconcileInternal(ctx, &apmServer)
	oldStatus := apmServer.Status.Association
	span, ctx = apm.StartSpan(ctx, "update_association", "app")
	if !reflect.DeepEqual(oldStatus, newStatus) {
		apmServer.Status.Association = newStatus
		if err := r.Status().Update(&apmServer); err != nil {
			return defaultRequeue, commonapm.CaptureError(ctx, err)
		}
		r.recorder.AnnotatedEventf(&apmServer,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)

	}
	span.End()
	return resultFromStatus(newStatus), err
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

func (r *ReconcileApmServerElasticsearchAssociation) isCompatible(apmServer *apmv1.ApmServer) (bool, error) {
	selector := map[string]string{labels.ApmServerNameLabelName: apmServer.Name}
	compat, err := annotation.ReconcileCompatibility(r.Client, apmServer, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileInternal(ctx context.Context, apmServer *apmv1.ApmServer) (commonv1.AssociationStatus, error) {
	// no auto-association nothing to do
	elasticsearchRef := apmServer.Spec.ElasticsearchRef
	if !elasticsearchRef.IsDefined() {
		return commonv1.AssociationUnknown, nil
	}
	if elasticsearchRef.Namespace == "" {
		// no namespace provided: default to the APM server namespace
		elasticsearchRef.Namespace = apmServer.Namespace
	}
	assocKey := k8s.ExtractNamespacedName(apmServer)
	// Make sure we see events from Elasticsearch using a dynamic watch
	// will become more relevant once we refactor user handling to CRDs and implement
	// syncing of user credentials across namespaces
	err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(assocKey),
		Watched: []types.NamespacedName{elasticsearchRef.NamespacedName()},
		Watcher: assocKey,
	})
	if err != nil {
		return commonv1.AssociationFailed, err
	}

	var es esv1.Elasticsearch
	var span *apm.Span
	span, ctx = apm.StartSpan(ctx, "get_elasticsearch", "app")
	err = r.Get(elasticsearchRef.NamespacedName(), &es)
	span.End()
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := association.RemoveAssociationConf(r.Client, apmServer); err != nil && !errors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch output from APMServer object", "namespace", apmServer.Namespace, "name", apmServer.Name)
				return commonv1.AssociationPending, apm.CaptureError(ctx, err)
			}

			return commonv1.AssociationPending, nil
		}
		return commonv1.AssociationFailed, apm.CaptureError(ctx, err)
	}

	span, ctx = apm.StartSpan(ctx, "reconcile_es_user", "app")
	if err := association.ReconcileEsUser(
		r.Client,
		r.scheme,
		apmServer,
		map[string]string{
			AssociationLabelName:      apmServer.Name,
			AssociationLabelNamespace: apmServer.Namespace,
		},
		"superuser",
		apmUserSuffix,
		es,
	); err != nil { // TODO distinguish conflicts and non-recoverable errors here
		return commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
	}
	span.End()

	span, ctx = apm.StartSpan(ctx, "reconcile_es_ca", "app")
	caSecret, err := r.reconcileElasticsearchCA(apmServer, elasticsearchRef.NamespacedName())
	if err != nil {
		return commonv1.AssociationPending, commonapm.CaptureError(ctx, err) // maybe not created yet
	}
	span.End()

	// construct the expected ES output configuration
	authSecretRef := association.ClearTextSecretKeySelector(apmServer, apmUserSuffix)
	expectedAssocConf := &commonv1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	span, ctx = apm.StartSpan(ctx, "update_apm_assoc", "apm")
	if !reflect.DeepEqual(expectedAssocConf, apmServer.AssociationConf()) {
		log.Info("Updating APMServer spec with Elasticsearch association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
		if err := association.UpdateAssociationConf(r.Client, apmServer, expectedAssocConf); err != nil {
			if errors.IsConflict(err) {
				return commonv1.AssociationPending, nil
			}
			log.Error(err, "Failed to update APMServer association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
			return commonv1.AssociationPending, commonapm.CaptureError(ctx, err)
		}
		apmServer.SetAssociationConf(expectedAssocConf)
	}
	span.End()

	span, ctx = apm.StartSpan(ctx, "delete_orphaned_resources", "app")
	if err := deleteOrphanedResources(r, apmServer); err != nil {
		log.Error(commonapm.CaptureError(ctx, err), "Error while trying to delete orphaned resources. Continuing.", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
	}
	span.End()

	return commonv1.AssociationEstablished, nil
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileElasticsearchCA(apm *apmv1.ApmServer, es types.NamespacedName) (association.CASecret, error) {
	apmKey := k8s.ExtractNamespacedName(apm)
	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(apmKey),
		Watched: []types.NamespacedName{http.PublicCertsSecretRef(esv1.ESNamer, es)},
		Watcher: apmKey,
	}); err != nil {
		return association.CASecret{}, err
	}
	// Build the labels applied on the secret
	labels := labels.NewLabels(apm.Name)
	labels[AssociationLabelName] = apm.Name
	return association.ReconcileCASecret(
		r.Client,
		r.scheme,
		apm,
		es,
		labels,
		elasticsearchCASecretSuffix,
	)
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association the standard reconcile mechanism will not delete the
// now redundant old user object/secret. This function lists all resources that don't match the current name/namespace
// combinations and deletes them.
func deleteOrphanedResources(c k8s.Client, apm *apmv1.ApmServer) error {
	var secrets corev1.SecretList
	ns := client.InNamespace(apm.Namespace)
	matchLabels := client.MatchingLabels(NewResourceLabels(apm.Name))
	if err := c.List(&secrets, ns, matchLabels); err != nil {
		return err
	}

	for _, s := range secrets.Items {
		controlledBy := metav1.IsControlledBy(&s, apm)
		if controlledBy && !apm.Spec.ElasticsearchRef.IsDefined() {
			log.Info("Deleting secret", "namespace", s.Namespace, "secret_name", s.Name, "as_name", apm.Name)
			if err := c.Delete(&s); err != nil {
				return err
			}
		}
	}
	return nil
}
