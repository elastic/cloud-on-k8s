// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	entsname "github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)


const (
	name = "enterprisesearch-controller"
)

var (
	log = logf.Log.WithName(name)
)

// Add creates a new EnterpriseSearch Controller and adds it to the Manager with default RBAC.
//The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileEnterpriseSearch {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileEnterpriseSearch{
		Client:         client,
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetEventRecorderFor(name),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileEnterpriseSearch) error {
	// Watch for changes to EnterpriseSearch
	err := c.Watch(&source.Kind{Type: &entsv1beta1.EnterpriseSearch{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &entsv1beta1.EnterpriseSearch{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &entsv1beta1.EnterpriseSearch{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &entsv1beta1.EnterpriseSearch{},
	}); err != nil {
		return err
	}

	return nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	return controller.New(name, mgr, controller.Options{Reconciler: r})
}

var _ reconcile.Reconciler = &ReconcileEnterpriseSearch{}


// ReconcileEnterpriseSearch reconciles an ApmServer object
type ReconcileEnterpriseSearch struct {
	k8s.Client
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileEnterpriseSearch) K8sClient() k8s.Client {
	return r.Client
}

func (r *ReconcileEnterpriseSearch) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

func (r *ReconcileEnterpriseSearch) Recorder() record.EventRecorder {
	return r.recorder
}

func (r *ReconcileEnterpriseSearch) Scheme() *runtime.Scheme {
	return r.scheme
}

var _ driver.Interface = &ReconcileEnterpriseSearch{}


// Reconcile reads that state of the cluster for an EnterpriseSearch object and makes changes based on the state read
// and what is in the EnterpriseSearch.Spec.
func (r *ReconcileEnterpriseSearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "ents_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "enterprisesearch")
	defer tracing.EndTransaction(tx)

	var ents entsv1beta1.EnterpriseSearch
	if err := association.FetchWithAssociation(ctx, r.Client, request, &ents); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsPaused(ents.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", ents.Namespace, "ents_name", ents.Name)
		return common.PauseRequeue, nil
	}

	if compatible, err := r.isCompatible(ctx, &ents); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if ents.IsMarkedForDeletion() {
		// Enterprise Search will be deleted, clean up resources
		r.onDelete(k8s.ExtractNamespacedName(&ents))
		return reconcile.Result{}, nil
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &ents, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if !association.IsConfiguredIfSet(&ents, r.recorder) {
		return reconcile.Result{}, nil
	}

	return r.doReconcile(ctx, request, ents)
}


func (r *ReconcileEnterpriseSearch) onDelete(obj types.NamespacedName) {
	// Clean up watches
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
}


func (r *ReconcileEnterpriseSearch) isCompatible(ctx context.Context, ents *entsv1beta1.EnterpriseSearch) (bool, error) {
	selector := map[string]string{EnterpriseSearchNameLabelName: ents.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, ents, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, ents, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}


func (r *ReconcileEnterpriseSearch) doReconcile(ctx context.Context, request reconcile.Request, ents entsv1beta1.EnterpriseSearch) (reconcile.Result, error) {
	state := NewState(request, &ents)

	svc, err := common.ReconcileService(ctx, r.Client, r.scheme, NewService(ents), &ents)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, err = ReconcileConfig(r.K8sClient(), r.Scheme(), ents)
	if err != nil {
		return reconcile.Result{}, err
	}

	// TODO: hash
	// TODO: certs
	//results := apmcerts.Reconcile(ctx, r, as, []corev1.Service{*svc}, r.CACertRotation)
	//if results.HasError() {
	//	res, err := results.Aggregate()
	//	k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
	//	return res, err
	//}

	state, err = r.reconcileDeployment(ctx, state, ents)
	if err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return reconcile.Result{Requeue: true}, nil
		}
		k8s.EmitErrorEvent(r.recorder, err, &ents, events.EventReconciliationError, "Deployment reconciliation error: %v", err)
		return state.Result, tracing.CaptureError(ctx, err)
	}

	state.UpdateEnterpriseSearchExternalService(*svc)

	//// update status
	//err = r.updateStatus(ctx, state)
	//if err != nil && errors.IsConflict(err) {
	//	log.V(1).Info("Conflict while updating status", "namespace", as.Namespace, "as", as.Name)
	//	return reconcile.Result{Requeue: true}, nil
	//}
	//res, err := results.WithError(err).Aggregate()
	//k8s.EmitErrorEvent(r.recorder, err, ents, events.EventReconciliationError, "Reconciliation error: %v", err)
	return reconcile.Result{}, nil
}


func NewService(ents entsv1beta1.EnterpriseSearch) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: ents.Spec.HTTP.Service.ObjectMeta,
		Spec:       ents.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = ents.Namespace
	svc.ObjectMeta.Name = entsname.HTTPService(ents.Name)

	labels := NewLabels(ents.Name)
	ports := []corev1.ServicePort{
		{
			Name:     ents.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
