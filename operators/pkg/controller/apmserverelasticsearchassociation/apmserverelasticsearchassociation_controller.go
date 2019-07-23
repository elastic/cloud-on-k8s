// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserverelasticsearchassociation

import (
	"reflect"
	"sync/atomic"
	"time"

	apmtype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	estype "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/user"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
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

const name = "apm-es-association-controller"

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
		recorder:   mgr.GetRecorder(name),
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
	iteration int64
}

// Reconcile reads that state of the cluster for a ApmServerElasticsearchAssociation object and makes changes based on the state read
// and what is in the ApmServerElasticsearchAssociation.Spec
func (r *ReconcileApmServerElasticsearchAssociation) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "namespace", request.Namespace, "as_name", request.Name)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime), "namespace", request.Namespace, "as_name", request.Name)
	}()

	var apmServer apmtype.ApmServer
	err := r.Get(request.NamespacedName, &apmServer)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if common.IsPaused(apmServer.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", apmServer.Namespace, "as_name", apmServer.Name, "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	handler := finalizer.NewHandler(r)
	err = handler.Handle(
		&apmServer,
		watchFinalizer(k8s.ExtractNamespacedName(&apmServer), r.watches),
		user.UserFinalizer(r.Client, NewUserLabelSelector(k8s.ExtractNamespacedName(&apmServer))),
	)
	if err != nil {
		// failed to prepare finalizer or run finalizer: retry
		return defaultRequeue, err
	}

	// ApmServer is being deleted short-circuit reconciliation
	if !apmServer.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	selector := k8slabels.Set(map[string]string{labels.ApmServerNameLabelName: apmServer.Name}).AsSelector()
	compat, err := annotation.ReconcileCompatibility(r.Client, &apmServer, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !compat {
		// this resource is not able to be reconciled by this version of the controller, so we will skip it and not requeue
		return reconcile.Result{}, nil
	}

	err = annotation.UpdateControllerVersion(r.Client, &apmServer, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		return reconcile.Result{}, err
	}

	newStatus, err := r.reconcileInternal(apmServer)
	// maybe update status
	origStatus := apmServer.Status.DeepCopy()
	apmServer.Status.Association = newStatus

	if !reflect.DeepEqual(*origStatus, apmServer.Status) {
		if err := r.Status().Update(&apmServer); err != nil {
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

// watchFinalizer ensure that we remove watches for Elasticsearch clusters that we are no longer interested in
// because the assocation to the APM server has been deleted.
func watchFinalizer(assocKey types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "dynamic-watches.finalizers.apm.k8s.elastic.co",
		Execute: func() error {
			w.ElasticsearchClusters.RemoveHandlerForKey(elasticsearchWatchName(assocKey))
			return nil
		},
	}
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

func (r *ReconcileApmServerElasticsearchAssociation) reconcileInternal(apmServer apmtype.ApmServer) (commonv1alpha1.AssociationStatus, error) {
	// no auto-association nothing to do
	elasticsearchRef := apmServer.Spec.ElasticsearchRef
	if !elasticsearchRef.IsDefined() {
		return "", nil
	}
	if elasticsearchRef.Namespace == "" {
		// no namespace provided: default to the APM server namespace
		elasticsearchRef.Namespace = apmServer.Namespace
	}
	assocKey := k8s.ExtractNamespacedName(&apmServer)
	// Make sure we see events from Elasticsearch using a dynamic watch
	// will become more relevant once we refactor user handling to CRDs and implement
	// syncing of user credentials across namespaces
	err := r.watches.ElasticsearchClusters.AddHandler(watches.NamedWatch{
		Name:    elasticsearchWatchName(assocKey),
		Watched: elasticsearchRef.NamespacedName(),
		Watcher: assocKey,
	})
	if err != nil {
		return commonv1alpha1.AssociationFailed, err
	}

	var es estype.Elasticsearch
	err = r.Get(elasticsearchRef.NamespacedName(), &es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Es not found, could be deleted or not yet created? Recheck in a while
			return commonv1alpha1.AssociationPending, nil
		}
		return commonv1alpha1.AssociationFailed, err
	}

	// TODO reconcile external user CRD here
	err = reconcileEsUser(r.Client, r.scheme, apmServer, es)
	if err != nil {
		return commonv1alpha1.AssociationPending, err // TODO distinguish conflicts and non-recoverable errors here
	}

	var expectedEsConfig apmtype.ElasticsearchOutput
	// TODO: look up public certs secret name from the ES cluster resource instead of relying on naming convention
	var publicCertsSecret corev1.Secret
	publicCertsSecretKey := http.PublicCertsSecretRef(
		esname.ESNamer,
		elasticsearchRef.NamespacedName(),
	)
	if err = r.Get(publicCertsSecretKey, &publicCertsSecret); err != nil {
		return commonv1alpha1.AssociationPending, err // maybe not created yet
	}
	// TODO this is currently limiting the association to the same namespace
	expectedEsConfig.SSL.CertificateAuthorities = commonv1alpha1.SecretRef{SecretName: publicCertsSecret.Name}
	expectedEsConfig.Hosts = []string{services.ExternalServiceURL(es)}
	expectedEsConfig.Auth.SecretKeyRef = clearTextSecretKeySelector(apmServer)

	// TODO: this is a bit rough
	if !reflect.DeepEqual(apmServer.Spec.Elasticsearch, expectedEsConfig) {
		apmServer.Spec.Elasticsearch = expectedEsConfig
		log.Info("Updating Apm Server spec with Elasticsearch output configuration", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
		if err := r.Update(&apmServer); err != nil {
			return commonv1alpha1.AssociationPending, err
		}
	}

	if err := deleteOrphanedResources(r, apmServer); err != nil {
		log.Error(err, "Error while trying to delete orphaned resources. Continuing.", "namespace", apmServer.Namespace, "as_name", apmServer.Name)
	}

	return commonv1alpha1.AssociationEstablished, nil
}

// deleteOrphanedResources deletes resources created by this association that are left over from previous reconciliation
// attempts. If a user changes namespace on a vertex of an association the standard reconcile mechanism will not delete the
// now redundant old user object/secret. This function lists all resources that don't match the current name/namespace
// combinations and deletes them.
func deleteOrphanedResources(c k8s.Client, apm apmtype.ApmServer) error {
	var secrets corev1.SecretList
	selector := NewResourceSelector(apm.Name)
	if err := c.List(&client.ListOptions{LabelSelector: selector}, &secrets); err != nil {
		return err
	}

	for _, s := range secrets.Items {
		controlledBy := metav1.IsControlledBy(&s, &apm)
		if controlledBy && !apm.Spec.ElasticsearchRef.IsDefined() {
			log.Info("Deleting secret", "namespace", s.Namespace, "secret_name", s.Name, "as_name", apm.Name)
			if err := c.Delete(&s); err != nil {
				return err
			}
		}
	}
	return nil
}
