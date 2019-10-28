// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"reflect"
	"time"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
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
	return &ReconcileApmServerElasticsearchAssociation{
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
		return nil, err
	}
	return c, nil
}

func addWatches(c controller.Controller, r *ReconcileApmServerElasticsearchAssociation) error {
	// Watch for changes to ApmServers
	if err := c.Watch(&source.Kind{Type: &apmtype.ApmServer{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch Elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &estype.Elasticsearch{}}, r.watches.ElasticsearchClusters); err != nil {
		return err
	}

	// Dynamically watch Elasticsearch public CA secrets for referenced ES clusters
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.watches.Secrets); err != nil {
		return err
	}

	// Watch Secrets owned by an ApmServer resource
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &apmtype.ApmServer{},
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
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

// Reconcile reads that state of the cluster for a ApmServerElasticsearchAssociation object and makes changes based on the state read
// and what is in the ApmServerElasticsearchAssociation.Spec
func (r *ReconcileApmServerElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, &r.iteration)()

	var apmServer apmtype.ApmServer
	if ok, err := association.FetchWithAssociation(r.Client, request, &apmServer); !ok {
		return reconcile.Result{}, err
	}

	if common.IsPaused(apmServer.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
		return common.PauseRequeue, nil
	}

	handler := finalizer.NewHandler(r)
	apmName := k8s.ExtractNamespacedName(&apmServer)
	err := handler.Handle(
		&apmServer,
		watchFinalizer(apmName, r.watches),
		user.UserFinalizer(r.Client, apmServer.Kind, NewUserLabelSelector(apmName)),
	)
	if err != nil {
		// failed to prepare finalizer or run finalizer: retry
		return defaultRequeue, err
	}

	// ApmServer is being deleted short-circuit reconciliation
	if !apmServer.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(&apmServer); err != nil || !compatible {
		return reconcile.Result{}, err
	}

	if err := annotation.UpdateControllerVersion(r.Client, &apmServer, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, err
	}

	newStatus, err := r.reconcileInternal(&apmServer)
	oldStatus := apmServer.Status.Association
	if !reflect.DeepEqual(oldStatus, newStatus) {
		apmServer.Status.Association = newStatus
		if err := r.Status().Update(&apmServer); err != nil {
			return defaultRequeue, err
		}
		r.recorder.AnnotatedEventf(&apmServer,
			annotation.ForAssociationStatusChange(oldStatus, newStatus),
			corev1.EventTypeNormal,
			events.EventAssociationStatusChange,
			"Association status changed from [%s] to [%s]", oldStatus, newStatus)

	}
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

// watchFinalizer ensure that we remove watches for Elasticsearch clusters that we are no longer interested in
// because the association to the APM server has been deleted.
func watchFinalizer(assocKey types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "finalizer.association.apmserver.k8s.elastic.co/elasticsearch",
		Execute: func() error {
			w.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(assocKey))
			w.Secrets.RemoveHandlerForKey(esCAWatchName(assocKey))
			return nil
		},
	}
}

func resultFromStatus(status commonv1beta1.AssociationStatus) reconcile.Result {
	switch status {
	case commonv1beta1.AssociationPending:
		return defaultRequeue // retry
	default:
		return reconcile.Result{} // we are done or there is not much we can do
	}
}

func (r *ReconcileApmServerElasticsearchAssociation) isCompatible(apmServer *apmtype.ApmServer) (bool, error) {
	selector := map[string]string{labels.ApmServerNameLabelName: apmServer.Name}
	compat, err := annotation.ReconcileCompatibility(r.Client, apmServer, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileInternal(apmServer *apmtype.ApmServer) (commonv1beta1.AssociationStatus, error) {
	// no auto-association nothing to do
	elasticsearchRef := apmServer.Spec.ElasticsearchRef
	if !elasticsearchRef.IsDefined() {
		return commonv1beta1.AssociationUnknown, nil
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
		return commonv1beta1.AssociationFailed, err
	}

	var es estype.Elasticsearch
	err = r.Get(elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, apmServer, events.EventAssociationError,
			"Failed to find referenced backend %s: %v", elasticsearchRef.NamespacedName(), err)
		if apierrors.IsNotFound(err) {
			// ES is not found, remove any existing backend configuration and retry in a bit.
			if err := association.RemoveAssociationConf(r.Client, apmServer); err != nil && !errors.IsConflict(err) {
				log.Error(err, "Failed to remove Elasticsearch output from APMServer object", "namespace", apmServer.Namespace, "name", apmServer.Name)
				return commonv1beta1.AssociationPending, err
			}

			return commonv1beta1.AssociationPending, nil
		}
		return commonv1beta1.AssociationFailed, err
	}

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
		return commonv1beta1.AssociationPending, err
	}

	caSecret, err := r.reconcileElasticsearchCA(apmServer, elasticsearchRef.NamespacedName())
	if err != nil {
		return commonv1beta1.AssociationPending, err // maybe not created yet
	}

	// construct the expected ES output configuration
	authSecretRef := association.ClearTextSecretKeySelector(apmServer, apmUserSuffix)
	expectedAssocConf := &commonv1beta1.AssociationConf{
		AuthSecretName: authSecretRef.Name,
		AuthSecretKey:  authSecretRef.Key,
		CACertProvided: caSecret.CACertProvided,
		CASecretName:   caSecret.Name,
		URL:            services.ExternalServiceURL(es),
	}

	if !reflect.DeepEqual(expectedAssocConf, apmServer.AssociationConf()) {
		log.Info("Updating APMServer spec with Elasticsearch association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
		if err := association.UpdateAssociationConf(r.Client, apmServer, expectedAssocConf); err != nil {
			if errors.IsConflict(err) {
				return commonv1beta1.AssociationPending, nil
			}
			log.Error(err, "Failed to update APMServer association configuration", "namespace", apmServer.Namespace, "name", apmServer.Name)
			return commonv1beta1.AssociationPending, err
		}
		apmServer.SetAssociationConf(expectedAssocConf)
	}

	if err := deleteOrphanedResources(r, apmServer); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
	}

	return commonv1beta1.AssociationEstablished, nil
}

func (r *ReconcileApmServerElasticsearchAssociation) reconcileElasticsearchCA(apm *apmtype.ApmServer, es types.NamespacedName) (association.CASecret, error) {
	apmKey := k8s.ExtractNamespacedName(apm)
	// watch ES CA secret to reconcile on any change
	if err := r.watches.Secrets.AddHandler(watches.NamedWatch{
		Name:    esCAWatchName(apmKey),
		Watched: []types.NamespacedName{http.PublicCertsSecretRef(estype.ESNamer, es)},
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
func deleteOrphanedResources(c k8s.Client, apm *apmtype.ApmServer) error {
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
