// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package maps

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"sync/atomic"
	"time"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

const (
	controllerName = "maps-controller"
)

var log = ulog.Log.WithName(controllerName)

// Add creates a new MapsServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileMapsServer {
	client := mgr.GetClient()
	return &ReconcileMapsServer{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		licenseChecker: license.NewLicenseChecker(client, params.OperatorNamespace),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileMapsServer) error {
	// Watch for changes to MapsServer
	if err := c.Watch(&source.Kind{Type: &emsv1alpha1.ElasticMapsServer{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &emsv1alpha1.ElasticMapsServer{},
	}); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(c, NameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &emsv1alpha1.ElasticMapsServer{},
	}); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &emsv1alpha1.ElasticMapsServer{},
	}); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(c, emsv1alpha1.Kind); err != nil {
		return err
	}

	// Dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets)
}

var _ reconcile.Reconciler = &ReconcileMapsServer{}

// ReconcileMapsServer reconciles a MapsServer object
type ReconcileMapsServer struct {
	k8s.Client
	operator.Parameters
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	licenseChecker license.Checker
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileMapsServer) K8sClient() k8s.Client {
	return r.Client
}

func (r *ReconcileMapsServer) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

func (r *ReconcileMapsServer) Recorder() record.EventRecorder {
	return r.recorder
}

var _ driver.Interface = &ReconcileMapsServer{}

// Reconcile reads that state of the cluster for a MapsServer object and makes changes based on the state read and what is
// in the MapsServer.Spec
func (r *ReconcileMapsServer) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(ctx, r.Tracer, request.NamespacedName, "maps")
	defer tracing.EndTransaction(tx)

	// retrieve the EMS object
	var ems emsv1alpha1.ElasticMapsServer
	if err := r.Client.Get(ctx, request.NamespacedName, &ems); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(&ems) {
		log.Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", ems.Namespace, "name", ems.Name)
		return reconcile.Result{}, nil
	}

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled()
	if err != nil {
		return reconcile.Result{}, err
	}

	if !enabled {
		msg := "Elastic Maps Server is an enterprise feature. Enterprise features are disabled"
		log.Info(msg, "namespace", ems.Namespace, "name", ems.Name)
		r.recorder.Eventf(&ems, corev1.EventTypeWarning, events.EventReconciliationError, msg)
		// we don't have a good way of watching for the license level to change so just requeue with a reasonably long delay
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Minute}, nil
	}

	// MapsServer will be deleted nothing to do other than remove the watches
	if ems.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(k8s.ExtractNamespacedName(&ems))
	}

	isEsAssocConfigured, err := association.IsConfiguredIfSet(&ems, r.recorder)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !isEsAssocConfigured {
		return reconcile.Result{}, nil
	}

	// main reconciliation logic
	return r.doReconcile(ctx, ems)
}

func (r *ReconcileMapsServer) doReconcile(ctx context.Context, ems emsv1alpha1.ElasticMapsServer) (reconcile.Result, error) {
	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, ems); err != nil {
		return reconcile.Result{}, err
	}

	svc, err := common.ReconcileService(ctx, r.Client, NewService(ems), &ems)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, results := certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Owner:                 &ems,
		TLSOptions:            ems.Spec.HTTP.TLS,
		Namer:                 EMSNamer,
		Labels:                labels(ems.Name),
		Services:              []corev1.Service{*svc},
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		res, err := results.Aggregate()
		k8s.EmitErrorEvent(r.recorder, err, &ems, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return res, err
	}

	emsVersion, err := version.Parse(ems.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}
	logger := log.WithValues("namespace", ems.Namespace, "maps_name", ems.Name) // TODO  mapping explosion
	assocAllowed, err := association.AllowVersion(emsVersion, ems.Associated(), logger, r.recorder)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !assocAllowed {
		return reconcile.Result{}, nil // will eventually retry once updated
	}

	configSecret, err := reconcileConfig(r, ems, r.IPFamily)
	if err != nil {
		return reconcile.Result{}, err
	}

	// build a hash of various inputs to rotate Pods on any change
	configHash, err := buildConfigHash(r.K8sClient(), ems, configSecret)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("build config hash: %w", err)
	}

	deploy, err := r.reconcileDeployment(ctx, ems, configHash)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("reconcile deployment: %w", err)
	}

	err = r.updateStatus(ems, deploy)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("updating status: %w", err)
	}

	return results.Aggregate()
}

func (r *ReconcileMapsServer) validate(ctx context.Context, ems emsv1alpha1.ElasticMapsServer) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if err := ems.ValidateCreate(); err != nil {
		log.Error(err, "Validation failed")
		k8s.EmitErrorEvent(r.recorder, err, &ems, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func NewService(ems emsv1alpha1.ElasticMapsServer) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: ems.Spec.HTTP.Service.ObjectMeta,
		Spec:       ems.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = ems.Namespace
	svc.ObjectMeta.Name = HTTPService(ems.Name)

	labels := labels(ems.Name)
	ports := []corev1.ServicePort{
		{
			Name:     ems.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}

func buildConfigHash(c k8s.Client, ems emsv1alpha1.ElasticMapsServer, configSecret corev1.Secret) (string, error) {
	// build a hash of various settings to rotate the Pod on any change
	configHash := fnv.New32a()

	// - in the Elastic Maps Server configuration file content
	_, _ = configHash.Write(configSecret.Data[ConfigFilename])

	// - in the Elastic Maps Server TLS certificates
	if ems.Spec.HTTP.TLS.Enabled() {
		var tlsCertSecret corev1.Secret
		tlsSecretKey := types.NamespacedName{Namespace: ems.Namespace, Name: certificates.InternalCertsSecretName(EMSNamer, ems.Name)}
		if err := c.Get(context.Background(), tlsSecretKey, &tlsCertSecret); err != nil {
			return "", err
		}
		if certPem, ok := tlsCertSecret.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(certPem)
		}
	}

	// - in the associated Elasticsearch TLS certificates
	if err := commonassociation.WriteAssocsToConfigHash(c, ems.GetAssociations(), configHash); err != nil {
		return "", err
	}

	return fmt.Sprint(configHash.Sum32()), nil
}

func (r *ReconcileMapsServer) reconcileDeployment(
	ctx context.Context,
	ems emsv1alpha1.ElasticMapsServer,
	configHash string,
) (appsv1.Deployment, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	deployParams, err := r.deploymentParams(ems, configHash)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	deploy := deployment.New(deployParams)
	return deployment.Reconcile(r.K8sClient(), deploy, &ems)
}

func (r *ReconcileMapsServer) deploymentParams(ems emsv1alpha1.ElasticMapsServer, configHash string) (deployment.Params, error) {
	podSpec, err := newPodSpec(ems, configHash)
	if err != nil {
		return deployment.Params{}, err
	}

	deploymentLabels := labels(ems.Name)

	podLabels := maps.Merge(labels(ems.Name), versionLabels(ems))
	// merge with user-provided labels
	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            Deployment(ems.Name),
		Namespace:       ems.Namespace,
		Replicas:        ems.Spec.Count,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
	}, nil
}

func (r *ReconcileMapsServer) updateStatus(ems emsv1alpha1.ElasticMapsServer, deploy appsv1.Deployment) error {
	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), ems.Namespace, map[string]string{NameLabelName: ems.Name})
	if err != nil {
		return err
	}
	deploymentStatus, err := common.DeploymentStatus(ems.Status.DeploymentStatus, deploy, pods, versionLabelName)
	if err != nil {
		return err
	}
	newStatus := emsv1alpha1.MapsStatus{
		DeploymentStatus:  deploymentStatus,
		AssociationStatus: ems.Status.AssociationStatus,
	}

	if reflect.DeepEqual(newStatus, ems.Status) {
		return nil // nothing to do
	}
	if newStatus.IsDegraded(ems.Status.DeploymentStatus) {
		r.recorder.Event(&ems, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elastic Maps Server health degraded")
	}
	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", ems.Namespace,
		"maps_name", ems.Name,
		"status", newStatus,
	)
	ems.Status = newStatus
	return common.UpdateStatus(r.Client, &ems)
}

func (r *ReconcileMapsServer) onDelete(obj types.NamespacedName) error {
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(EMSNamer, obj.Name))
	// same for the configRef secret
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(r.Client, obj, emsv1alpha1.Kind)
}
