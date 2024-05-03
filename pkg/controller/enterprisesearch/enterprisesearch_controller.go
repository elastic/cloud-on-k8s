// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

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

	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName = "enterprisesearch-controller"
)

// Add creates a new EnterpriseSearch Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, controllerName, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileEnterpriseSearch {
	client := mgr.GetClient()
	return &ReconcileEnterpriseSearch{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileEnterpriseSearch) error {
	// Watch for changes to EnterpriseSearch
	err := c.Watch(source.Kind(mgr.GetCache(), &entv1.EnterpriseSearch{}, &handler.TypedEnqueueRequestForObject[*entv1.EnterpriseSearch]{}))
	if err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&entv1.EnterpriseSearch{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, EnterpriseSearchNameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&entv1.EnterpriseSearch{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&entv1.EnterpriseSearch{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, entv1.Kind); err != nil {
		return err
	}

	// Dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcileEnterpriseSearch{}

// ReconcileEnterpriseSearch reconciles an ApmServer object
type ReconcileEnterpriseSearch struct {
	k8s.Client
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

var _ driver.Interface = &ReconcileEnterpriseSearch{}

// Reconcile reads that state of the cluster for an EnterpriseSearch object and makes changes based on the state read
// and what is in the EnterpriseSearch.Spec.
func (r *ReconcileEnterpriseSearch) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "ent_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	var ent entv1.EnterpriseSearch
	if err := r.Client.Get(ctx, request.NamespacedName, &ent); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx,
				types.NamespacedName{
					Namespace: request.Namespace,
					Name:      request.Name,
				})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &ent) {
		ulog.FromContext(ctx).Info("Object is currently not managed by this controller. Skipping reconciliation", "namespace", ent.Namespace, "ent_name", ent.Name)
		return reconcile.Result{}, nil
	}

	results, status := r.doReconcile(ctx, ent)
	if err := r.updateStatus(ctx, ent, status); err != nil {
		if apierrors.IsConflict(err) {
			return results.WithResult(reconcile.Result{Requeue: true}).Aggregate()
		}
		results.WithError(err)
	}
	return results.Aggregate()
}

func (r *ReconcileEnterpriseSearch) onDelete(ctx context.Context, obj types.NamespacedName) error {
	// Clean up watches
	r.dynamicWatches.Secrets.RemoveHandlerForKey(common.ConfigRefWatchName(obj))
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(entv1.Namer, obj.Name))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, entv1.Kind)
}

func (r *ReconcileEnterpriseSearch) doReconcile(ctx context.Context, ent entv1.EnterpriseSearch) (*reconciler.Results, entv1.EnterpriseSearchStatus) {
	results := reconciler.NewResult(ctx)
	status := newStatus(ent)

	isEsAssocConfigured, err := association.IsConfiguredIfSet(ctx, &ent, r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !isEsAssocConfigured {
		return results, status
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, &ent); err != nil {
		return results.WithError(err), status
	}

	svc, err := common.ReconcileService(ctx, r.Client, NewService(ent), &ent)
	if err != nil {
		return results.WithError(err), status
	}

	_, results = certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Owner:                 &ent,
		TLSOptions:            ent.Spec.HTTP.TLS,
		Namer:                 entv1.Namer,
		Labels:                ent.GetIdentityLabels(),
		Services:              []corev1.Service{*svc},
		GlobalCA:              r.GlobalCA,
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(r.recorder, err, &ent, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results, status
	}

	entVersion, err := version.Parse(ent.Spec.Version)
	if err != nil {
		return results.WithError(err), status
	}
	assocAllowed, err := association.AllowVersion(entVersion, ent.Associated(), ulog.FromContext(ctx), r.recorder)
	if err != nil {
		return results.WithError(err), status
	}
	if !assocAllowed {
		return results, status // will eventually retry once updated
	}

	configSecret, err := ReconcileConfig(ctx, r, ent, r.IPFamily)
	if err != nil {
		return results.WithError(err), status
	}

	// toggle read-only mode for Enterprise Search version upgrades
	upgrade := VersionUpgrade{k8sClient: r.K8sClient(), recorder: r.Recorder(), ent: ent, dialer: r.Dialer}
	if err := upgrade.Handle(ctx); err != nil {
		return results.WithError(fmt.Errorf("version upgrade: %w", err)), status
	}

	// build a hash of various inputs to rotate Pods on any change
	configHash, err := buildConfigHash(r.K8sClient(), ent, configSecret)
	if err != nil {
		return results.WithError(fmt.Errorf("build config hash: %w", err)), status
	}

	deploy, err := r.reconcileDeployment(ctx, ent, configHash)
	if err != nil {
		return results.WithError(fmt.Errorf("reconcile deployment: %w", err)), status
	}

	status, err = r.generateStatus(ctx, ent, deploy, svc.Name)
	if err != nil {
		return results.WithError(fmt.Errorf("updating status: %w", err)), status
	}

	return results, status
}

// newStatus will generate a new status, ensuring status.ObservedGeneration
// follows the generation of the Enterprise Search object.
func newStatus(ent entv1.EnterpriseSearch) entv1.EnterpriseSearchStatus {
	status := ent.Status
	status.ObservedGeneration = ent.Generation
	return status
}

func (r *ReconcileEnterpriseSearch) validate(ctx context.Context, ent *entv1.EnterpriseSearch) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := ent.ValidateCreate(); err != nil {
		ulog.FromContext(ctx).Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, ent, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileEnterpriseSearch) generateStatus(ctx context.Context, ent entv1.EnterpriseSearch, deploy appsv1.Deployment, svcName string) (entv1.EnterpriseSearchStatus, error) {
	status := entv1.EnterpriseSearchStatus{
		Association:        ent.Status.Association,
		ExternalService:    svcName,
		ObservedGeneration: ent.Generation,
	}

	pods, err := k8s.PodsMatchingLabels(r.K8sClient(), ent.Namespace, map[string]string{EnterpriseSearchNameLabelName: ent.Name})
	if err != nil {
		return status, err
	}
	status.DeploymentStatus, err = common.DeploymentStatus(ctx, ent.Status.DeploymentStatus, deploy, pods, VersionLabelName)
	return status, err
}

func (r *ReconcileEnterpriseSearch) updateStatus(ctx context.Context, ent entv1.EnterpriseSearch, status entv1.EnterpriseSearchStatus) error {
	if reflect.DeepEqual(status, ent.Status) {
		return nil // nothing to do
	}
	if status.IsDegraded(ent.Status.DeploymentStatus) {
		r.recorder.Event(&ent, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Enterprise Search health degraded")
	}
	ulog.FromContext(ctx).V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", ent.Namespace,
		"ent_name", ent.Name,
		"status", status,
	)
	ent.Status = status
	return common.UpdateStatus(ctx, r.Client, &ent)
}

func NewService(ent entv1.EnterpriseSearch) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: ent.Spec.HTTP.Service.ObjectMeta,
		Spec:       ent.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = ent.Namespace
	svc.ObjectMeta.Name = HTTPServiceName(ent.Name)

	labels := ent.GetIdentityLabels()
	ports := []corev1.ServicePort{
		{
			Name:     ent.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}

func buildConfigHash(c k8s.Client, ent entv1.EnterpriseSearch, configSecret corev1.Secret) (string, error) {
	// build a hash of various settings to rotate the Pod on any change
	configHash := fnv.New32a()

	// - in the Enterprise Search configuration file content
	_, _ = configHash.Write(configSecret.Data[ConfigFilename])
	// - in the readiness probe script content
	_, _ = configHash.Write(configSecret.Data[ReadinessProbeFilename])

	// - in the Enterprise Search TLS certificates
	if ent.Spec.HTTP.TLS.Enabled() {
		var tlsCertSecret corev1.Secret
		tlsSecretKey := types.NamespacedName{Namespace: ent.Namespace, Name: certificates.InternalCertsSecretName(entv1.Namer, ent.Name)}
		if err := c.Get(context.Background(), tlsSecretKey, &tlsCertSecret); err != nil {
			return "", err
		}
		if certPem, ok := tlsCertSecret.Data[certificates.CertFileName]; ok {
			_, _ = configHash.Write(certPem)
		}
	}

	// - in the associated Elasticsearch TLS certificates
	if err := commonassociation.WriteAssocsToConfigHash(c, ent.GetAssociations(), configHash); err != nil {
		return "", err
	}

	return fmt.Sprint(configHash.Sum32()), nil
}
