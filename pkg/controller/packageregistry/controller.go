// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package packageregistry

import (
	"context"
	"fmt"
	"hash/fnv"
	"reflect"
	"sync/atomic"

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

	eprv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/packageregistry/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/packageregistry/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v3/pkg/utils/log"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
)

const (
	controllerName = "packageregistry-controller"
)

// Add creates a new PackageRegistry Controller and adds it to the Manager with default RBAC. The manager will set fields on the Controller
// and start it when the manager is started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcilePackageRegistry {
	return &ReconcilePackageRegistry{
		Client:         mgr.GetClient(),
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcilePackageRegistry) error {
	// Watch for changes to packageregistry
	if err := c.Watch(source.Kind(mgr.GetCache(), &eprv1alpha1.PackageRegistry{}, &handler.TypedEnqueueRequestForObject[*eprv1alpha1.PackageRegistry]{})); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&eprv1alpha1.PackageRegistry{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, label.NameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&eprv1alpha1.PackageRegistry{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&eprv1alpha1.PackageRegistry{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, eprv1alpha1.Kind); err != nil {
		return err
	}

	// Dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcilePackageRegistry{}

// ReconcilePackageRegistry reconciles a PackageRegistry object
type ReconcilePackageRegistry struct {
	k8s.Client
	operator.Parameters
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcilePackageRegistry) K8sClient() k8s.Client {
	return r.Client
}

func (r *ReconcilePackageRegistry) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

func (r *ReconcilePackageRegistry) Recorder() record.EventRecorder {
	return r.recorder
}

var _ driver.Interface = &ReconcilePackageRegistry{}

// Reconcile reads that state of the cluster for a PackageRegistry object and makes changes based on the state read and what is
// in the PackageRegistry.Spec
func (r *ReconcilePackageRegistry) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "epr_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	// retrieve the epr object
	var epr eprv1alpha1.PackageRegistry
	if err := r.Client.Get(ctx, request.NamespacedName, &epr); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &epr) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", epr.Namespace, "epr_name", epr.Name)
		return reconcile.Result{}, nil
	}

	// PackageRegistry will be deleted nothing to do other than remove the watches
	if epr.IsMarkedForDeletion() {
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&epr))
	}

	// main reconciliation logic
	results, status := r.doReconcile(ctx, epr)
	if err := r.updateStatus(ctx, epr, status); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithRequeue().Aggregate()
		}
		results.WithError(err)
	}
	return results.Aggregate()
}

func (r *ReconcilePackageRegistry) doReconcile(ctx context.Context, epr eprv1alpha1.PackageRegistry) (*reconciler.Results, eprv1alpha1.PackageRegistryStatus) {
	results := reconciler.NewResult(ctx)
	status := newStatus(epr)

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, epr); err != nil {
		return results.WithError(err), status
	}

	// extract the metadata that should be propagated to children
	meta := metadata.Propagate(&epr, metadata.Metadata{Labels: epr.GetIdentityLabels()})

	svc, err := common.ReconcileService(ctx, r.Client, NewService(epr, meta), &epr)
	if err != nil {
		return results.WithError(err), status
	}

	_, results = certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Owner:                 &epr,
		TLSOptions:            epr.Spec.HTTP.TLS,
		Namer:                 eprv1alpha1.Namer,
		Metadata:              meta,
		Services:              []corev1.Service{*svc},
		GlobalCA:              r.GlobalCA,
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(r.recorder, err, &epr, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results, status
	}

	configSecret, err := reconcileConfig(ctx, r, epr, meta)
	if err != nil {
		return results.WithError(err), status
	}

	// build a hash of various inputs to rotate Pods on any change
	configHash, err := buildConfigHash(ctx, r.K8sClient(), epr, configSecret)
	if err != nil {
		return results.WithError(fmt.Errorf("build config hash: %w", err)), status
	}

	deploy, err := r.reconcileDeployment(ctx, epr, configHash, meta)
	if err != nil {
		return results.WithError(fmt.Errorf("reconcile deployment: %w", err)), status
	}

	status, err = r.getStatus(ctx, epr, deploy)
	if err != nil {
		return results.WithError(fmt.Errorf("calculating status: %w", err)), status
	}

	return results, status
}

func newStatus(epr eprv1alpha1.PackageRegistry) eprv1alpha1.PackageRegistryStatus {
	status := epr.Status
	status.ObservedGeneration = epr.Generation
	return status
}

func (r *ReconcilePackageRegistry) validate(ctx context.Context, epr eprv1alpha1.PackageRegistry) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := epr.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, &epr, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func NewService(epr eprv1alpha1.PackageRegistry, meta metadata.Metadata) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: epr.Spec.HTTP.Service.ObjectMeta,
		Spec:       epr.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = epr.Namespace
	svc.ObjectMeta.Name = HTTPServiceName(epr.Name)

	selector := epr.GetIdentityLabels()
	ports := []corev1.ServicePort{
		{
			Name:     epr.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, meta, selector, ports)
}

func buildConfigHash(ctx context.Context, c k8s.Client, epr eprv1alpha1.PackageRegistry, configSecret corev1.Secret) (string, error) {
	// build a hash of various settings to rotate the Pod on any change
	configHash := fnv.New32a()

	// - in the Elastic Package Registry configuration file content
	_, _ = configHash.Write(configSecret.Data[ConfigFilename])

	// - in the Elastic Package Registry TLS certificates
	if epr.Spec.HTTP.TLS.Enabled() {
		var tlsCertSecret corev1.Secret
		tlsSecretKey := types.NamespacedName{Namespace: epr.Namespace, Name: certificates.InternalCertsSecretName(eprv1alpha1.Namer, epr.Name)}
		if err := c.Get(ctx, tlsSecretKey, &tlsCertSecret); err != nil {
			return "", err
		}
		if certPem, ok := tlsCertSecret.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(certPem)
		}
	}

	return fmt.Sprint(configHash.Sum32()), nil
}

func (r *ReconcilePackageRegistry) reconcileDeployment(
	ctx context.Context,
	epr eprv1alpha1.PackageRegistry,
	configHash string,
	meta metadata.Metadata,
) (appsv1.Deployment, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	deployParams, err := r.deploymentParams(epr, configHash, meta)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	deploy := deployment.New(deployParams)
	return deployment.Reconcile(ctx, r.K8sClient(), deploy, &epr)
}

func (r *ReconcilePackageRegistry) deploymentParams(epr eprv1alpha1.PackageRegistry, configHash string, meta metadata.Metadata) (deployment.Params, error) {
	podSpec, err := newPodSpec(epr, configHash, meta)
	if err != nil {
		return deployment.Params{}, err
	}

	deploymentLabels := epr.GetIdentityLabels()

	podLabels := maps.Merge(epr.GetIdentityLabels(), versionLabels(epr))
	// merge with user-provided labels
	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:                 DeploymentName(epr.Name),
		Namespace:            epr.Namespace,
		Replicas:             epr.Spec.Count,
		Selector:             deploymentLabels,
		Metadata:             meta,
		PodTemplateSpec:      podSpec,
		RevisionHistoryLimit: epr.Spec.RevisionHistoryLimit,
		Strategy:             appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
	}, nil
}

func (r *ReconcilePackageRegistry) getStatus(ctx context.Context, epr eprv1alpha1.PackageRegistry, deploy appsv1.Deployment) (eprv1alpha1.PackageRegistryStatus, error) {
	status := newStatus(epr)
	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), epr.Namespace, map[string]string{label.NameLabelName: epr.Name})
	if err != nil {
		return status, err
	}
	deploymentStatus, err := common.DeploymentStatus(ctx, epr.Status.DeploymentStatus, deploy, pods, label.VersionLabelName)
	if err != nil {
		return status, err
	}
	status.DeploymentStatus = deploymentStatus

	return status, nil
}

func (r *ReconcilePackageRegistry) updateStatus(ctx context.Context, epr eprv1alpha1.PackageRegistry, status eprv1alpha1.PackageRegistryStatus) error {
	if reflect.DeepEqual(status, epr.Status) {
		return nil // nothing to do
	}
	if status.IsDegraded(epr.Status.DeploymentStatus) {
		r.recorder.Event(&epr, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elastic Package Registry health degraded")
	}
	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", epr.Namespace,
		"epr_name", epr.Name,
		"status", status,
	)
	epr.Status = status
	return common.UpdateStatus(ctx, r.Client, &epr)
}

func (r *ReconcilePackageRegistry) onDelete(ctx context.Context, obj types.NamespacedName) error {
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(eprv1alpha1.Namer, obj.Name))
	// same for the configRef secret
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, eprv1alpha1.Kind)
}

func versionLabels(epr eprv1alpha1.PackageRegistry) map[string]string {
	return map[string]string{
		label.VersionLabelName: epr.Spec.Version,
	}
}
