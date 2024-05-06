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

	"go.elastic.co/apm/v2"
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

	emsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/maps/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

const (
	controllerName = "maps-controller"
)

// Add creates a new MapsServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, reconciler)
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

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileMapsServer) error {
	// Watch for changes to MapsServer
	if err := c.Watch(source.Kind(mgr.GetCache(), &emsv1alpha1.ElasticMapsServer{}, &handler.TypedEnqueueRequestForObject[*emsv1alpha1.ElasticMapsServer]{})); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&emsv1alpha1.ElasticMapsServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, NameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&emsv1alpha1.ElasticMapsServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&emsv1alpha1.ElasticMapsServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, emsv1alpha1.Kind); err != nil {
		return err
	}

	// Dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
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
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "maps_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the EMS object
	var ems emsv1alpha1.ElasticMapsServer
	if err := r.Client.Get(ctx, request.NamespacedName, &ems); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &ems) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", ems.Namespace, "maps_name", ems.Name)
		return reconcile.Result{}, nil
	}

	// MapsServer will be deleted nothing to do other than remove the watches
	if ems.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&ems))
	}

	// main reconciliation logic
	results, status := r.doReconcile(ctx, ems)
	if err := r.updateStatus(ctx, ems, status); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		results.WithError(err)
	}
	return results.Aggregate()
}

func (r *ReconcileMapsServer) doReconcile(ctx context.Context, ems emsv1alpha1.ElasticMapsServer) (*reconciler.Results, emsv1alpha1.MapsStatus) {
	log := ulog.FromContext(ctx)
	results := reconciler.NewResult(ctx)
	status := newStatus(ems)

	enabled, err := r.licenseChecker.EnterpriseFeaturesEnabled(ctx)
	if err != nil {
		return results.WithError(err), status
	}

	if !enabled {
		msg := "Elastic Maps Server is an enterprise feature. Enterprise features are disabled"
		log.Info(msg, "namespace", ems.Namespace, "maps_name", ems.Name)
		r.recorder.Eventf(&ems, corev1.EventTypeWarning, events.EventReconciliationError, msg)
		// we don't have a good way of watching for the license level to change so just requeue with a reasonably long delay
		return results.WithResult(reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Minute}), status
	}

	isEsAssocConfigured, err := association.IsConfiguredIfSet(ctx, &ems, r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !isEsAssocConfigured {
		return results, status
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, ems); err != nil {
		return results.WithError(err), status
	}

	svc, err := common.ReconcileService(ctx, r.Client, NewService(ems), &ems)
	if err != nil {
		return results.WithError(err), status
	}

	_, results = certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Owner:                 &ems,
		TLSOptions:            ems.Spec.HTTP.TLS,
		Namer:                 EMSNamer,
		Labels:                ems.GetIdentityLabels(),
		Services:              []corev1.Service{*svc},
		GlobalCA:              r.GlobalCA,
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(r.recorder, err, &ems, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results, status
	}

	emsVersion, err := version.Parse(ems.Spec.Version)
	if err != nil {
		return results.WithError(err), status
	}
	assocAllowed, err := association.AllowVersion(emsVersion, ems.Associated(), log, r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !assocAllowed {
		// will eventually retry once updated, along with the results
		// from the certificate reconciliation having a retry after a time period
		return results, status
	}

	configSecret, err := reconcileConfig(ctx, r, ems, r.IPFamily)
	if err != nil {
		return results.WithError(err), status
	}

	// build a hash of various inputs to rotate Pods on any change
	configHash, err := buildConfigHash(r.K8sClient(), ems, configSecret)
	if err != nil {
		return results.WithError(fmt.Errorf("build config hash: %w", err)), status
	}

	deploy, err := r.reconcileDeployment(ctx, ems, configHash)
	if err != nil {
		return results.WithError(fmt.Errorf("reconcile deployment: %w", err)), status
	}

	status, err = r.getStatus(ctx, ems, deploy)
	if err != nil {
		return results.WithError(fmt.Errorf("calculating status: %w", err)), status
	}

	return results, status
}

func newStatus(ems emsv1alpha1.ElasticMapsServer) emsv1alpha1.MapsStatus {
	status := ems.Status
	status.ObservedGeneration = ems.Generation
	return status
}

func (r *ReconcileMapsServer) validate(ctx context.Context, ems emsv1alpha1.ElasticMapsServer) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := ems.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, &ems, events.EventReasonValidation, err.Error())
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

	labels := ems.GetIdentityLabels()
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
	return deployment.Reconcile(ctx, r.K8sClient(), deploy, &ems)
}

func (r *ReconcileMapsServer) deploymentParams(ems emsv1alpha1.ElasticMapsServer, configHash string) (deployment.Params, error) {
	podSpec, err := newPodSpec(ems, configHash)
	if err != nil {
		return deployment.Params{}, err
	}

	deploymentLabels := ems.GetIdentityLabels()

	podLabels := maps.Merge(ems.GetIdentityLabels(), versionLabels(ems))
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

func (r *ReconcileMapsServer) getStatus(ctx context.Context, ems emsv1alpha1.ElasticMapsServer, deploy appsv1.Deployment) (emsv1alpha1.MapsStatus, error) {
	status := newStatus(ems)
	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), ems.Namespace, map[string]string{NameLabelName: ems.Name})
	if err != nil {
		return status, err
	}
	deploymentStatus, err := common.DeploymentStatus(ctx, ems.Status.DeploymentStatus, deploy, pods, versionLabelName)
	if err != nil {
		return status, err
	}
	status.DeploymentStatus = deploymentStatus
	status.AssociationStatus = ems.Status.AssociationStatus

	return status, nil
}

func (r *ReconcileMapsServer) updateStatus(ctx context.Context, ems emsv1alpha1.ElasticMapsServer, status emsv1alpha1.MapsStatus) error {
	if reflect.DeepEqual(status, ems.Status) {
		return nil // nothing to do
	}
	if status.IsDegraded(ems.Status.DeploymentStatus) {
		r.recorder.Event(&ems, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elastic Maps Server health degraded")
	}
	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", ems.Namespace,
		"maps_name", ems.Name,
		"status", status,
	)
	ems.Status = status
	return common.UpdateStatus(ctx, r.Client, &ems)
}

func (r *ReconcileMapsServer) onDelete(ctx context.Context, obj types.NamespacedName) error {
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(EMSNamer, obj.Name))
	// same for the configRef secret
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, emsv1alpha1.Kind)
}
