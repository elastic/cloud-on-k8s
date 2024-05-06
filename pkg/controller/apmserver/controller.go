// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmserver

import (
	"context"
	"path/filepath"
	"reflect"
	"sync/atomic"

	"go.elastic.co/apm/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	controllerName           = "apmserver-controller"
	configHashAnnotationName = "apm.k8s.elastic.co/config-hash"

	// ApmBaseDir is the base directory of the APM server
	ApmBaseDir = "/usr/share/apm-server"
)

var (
	log = ulog.Log.WithName(controllerName)

	// ApmServerBin is the apm server binary file
	ApmServerBin = filepath.Join(ApmBaseDir, "apm-server")

	initContainerParameters = keystore.InitContainerParameters{
		KeystoreCreateCommand:         ApmServerBin + " keystore create --force",
		KeystoreAddCommand:            ApmServerBin + ` keystore add "$key" --stdin < "$filename"`,
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		KeystoreVolumePath:            DataVolumePath,
		Resources: corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
		},
	}
)

// Add creates a new ApmServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileApmServer {
	return &ReconcileApmServer{
		Client:         mgr.GetClient(),
		recorder:       mgr.GetEventRecorderFor(controllerName),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(mgr manager.Manager, c controller.Controller, r *ReconcileApmServer) error {
	// Watch for changes to ApmServer
	err := c.Watch(source.Kind(mgr.GetCache(), &apmv1.ApmServer{}, &handler.TypedEnqueueRequestForObject[*apmv1.ApmServer]{}))
	if err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}, handler.TypedEnqueueRequestForOwner[*appsv1.Deployment](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&apmv1.ApmServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch Pods, to ensure `status.version` and version upgrades are correctly reconciled on any change.
	// Watching Deployments only may lead to missing some events.
	if err := watches.WatchPods(mgr, c, ApmServerNameLabelName); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}, handler.TypedEnqueueRequestForOwner[*corev1.Service](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&apmv1.ApmServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}

	// Watch owned and soft-owned secrets
	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, handler.TypedEnqueueRequestForOwner[*corev1.Secret](
		mgr.GetScheme(), mgr.GetRESTMapper(),
		&apmv1.ApmServer{}, handler.OnlyControllerOwner(),
	))); err != nil {
		return err
	}
	if err := watches.WatchSoftOwnedSecrets(mgr, c, apmv1.Kind); err != nil {
		return err
	}

	// dynamically watch referenced secrets to connect to Elasticsearch
	return c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{}, r.dynamicWatches.Secrets))
}

var _ reconcile.Reconciler = &ReconcileApmServer{}

// ReconcileApmServer reconciles an ApmServer object
type ReconcileApmServer struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its reconcile method
	iteration uint64
}

// K8sClient returns the kubernetes client from the APM Server reconciler.
func (r *ReconcileApmServer) K8sClient() k8s.Client {
	return r.Client
}

// DynamicWatches returns the set of dynamic watches from the APM Server reconciler.
func (r *ReconcileApmServer) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

// Recorder returns the Kubernetes recorder that is responsible for recording and reporting
// events from the APM Server reconciler.
func (r *ReconcileApmServer) Recorder() record.EventRecorder {
	return r.recorder
}

var _ driver.Interface = &ReconcileApmServer{}

// Reconcile reads that state of the cluster for a ApmServer object and makes changes based on the state read
// and what is in the ApmServer.Spec
func (r *ReconcileApmServer) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, controllerName, "as_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	var as apmv1.ApmServer
	if err := r.Client.Get(ctx, request.NamespacedName, &as); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, r.onDelete(ctx, types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(ctx, &as) {
		log.Info("Object currently not managed by this controller. Skipping reconciliation", "namespace", as.Namespace, "as_name", as.Name)
		return reconcile.Result{}, nil
	}

	// Remove any previous finalizer used in ECK v1.0.0-beta1 that we don't need anymore
	if err := finalizer.RemoveAll(ctx, r.Client, &as); err != nil {
		return reconcile.Result{}, err
	}

	if as.IsMarkedForDeletion() {
		// APM server will be deleted, clean up resources
		return reconcile.Result{}, r.onDelete(ctx, k8s.ExtractNamespacedName(&as))
	}

	results, state := r.doReconcile(ctx, &as)

	return results.WithError(r.updateStatus(ctx, state)).Aggregate()
}

func (r *ReconcileApmServer) doReconcile(ctx context.Context, as *apmv1.ApmServer) (*reconciler.Results, State) {
	state := NewState(as)
	results := reconciler.NewResult(ctx)

	areAssocsConfigured, err := association.AreConfiguredIfSet(ctx, as.GetAssociations(), r.recorder)
	if err != nil {
		return results.WithError(tracing.CaptureError(ctx, err)), state
	}
	if !areAssocsConfigured {
		return results, state
	}

	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, as); err != nil {
		return results.WithError(err), state
	}

	svc, err := common.ReconcileService(ctx, r.Client, NewService(*as), as)
	if err != nil {
		return results.WithError(err), state
	}

	_, results = certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Owner:                 as,
		TLSOptions:            as.Spec.HTTP.TLS,
		Namer:                 Namer,
		Labels:                as.GetIdentityLabels(),
		Services:              []corev1.Service{*svc},
		GlobalCA:              r.GlobalCA,
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		_, err := results.Aggregate()
		k8s.MaybeEmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return results, state
	}

	asVersion, err := version.Parse(as.Spec.Version)
	if err != nil {
		return results.WithError(err), state
	}
	logger := log.WithValues("namespace", as.Namespace, "as_name", as.Name)
	assocAllowed, err := association.AllowVersion(asVersion, as, logger, r.recorder)
	if err != nil {
		return results.WithError(tracing.CaptureError(ctx, err)), state
	}
	if !assocAllowed {
		return results, state // will eventually retry
	}

	state, err = r.reconcileApmServerDeployment(ctx, state, as, asVersion)
	if err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return results.WithResult(reconcile.Result{Requeue: true}), state
		}
		k8s.MaybeEmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Deployment reconciliation error: %v", err)
		return results.WithError(tracing.CaptureError(ctx, err)), state
	}

	state.UpdateApmServerExternalService(*svc)

	_, err = results.WithError(err).Aggregate()
	k8s.MaybeEmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Reconciliation error: %v", err)
	return results, state
}

func (r *ReconcileApmServer) validate(ctx context.Context, as *apmv1.ApmServer) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if _, err := as.ValidateCreate(); err != nil {
		log.Error(err, "Validation failed")
		k8s.MaybeEmitErrorEvent(r.recorder, err, as, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileApmServer) onDelete(ctx context.Context, obj types.NamespacedName) error {
	// Clean up watches set on secure settings
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
	// Clean up watches set on custom http tls certificates
	r.dynamicWatches.Secrets.RemoveHandlerForKey(certificates.CertificateWatchKey(Namer, obj.Name))
	return reconciler.GarbageCollectSoftOwnedSecrets(ctx, r.Client, obj, apmv1.Kind)
}

// reconcileApmServerToken reconciles a Secret containing the APM Server token.
// It reuses the existing token if possible.
func reconcileApmServerToken(ctx context.Context, c k8s.Client, as *apmv1.ApmServer) (corev1.Secret, error) {
	expectedApmServerSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      SecretToken(as.Name),
			Labels:    labels.AddCredentialsLabel(as.GetIdentityLabels()),
		},
		Data: make(map[string][]byte),
	}
	// reuse the secret token if it already exists
	var existingSecret corev1.Secret
	err := c.Get(ctx, k8s.ExtractNamespacedName(&expectedApmServerSecret), &existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return corev1.Secret{}, err
	}
	if token, exists := existingSecret.Data[SecretTokenKey]; exists {
		expectedApmServerSecret.Data[SecretTokenKey] = token
	} else {
		expectedApmServerSecret.Data[SecretTokenKey] = common.RandomBytes(24)
	}

	// Don't set an ownerRef for the APM token secret, likely to be copied into different namespaces.
	// See https://github.com/elastic/cloud-on-k8s/issues/3986.
	return reconciler.ReconcileSecretNoOwnerRef(ctx, c, expectedApmServerSecret, as)
}

func (r *ReconcileApmServer) updateStatus(ctx context.Context, state State) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	original := state.originalApmServer
	if reflect.DeepEqual(original.Status, state.ApmServer.Status) {
		return nil
	}
	if state.ApmServer.Status.IsDegraded(original.Status.DeploymentStatus) {
		r.recorder.Event(original, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Apm Server health degraded")
	}
	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", state.ApmServer.Namespace,
		"as_name", state.ApmServer.Name,
		"status", state.ApmServer.Status,
	)
	return common.UpdateStatus(ctx, r.Client, state.ApmServer)
}

// NewService returns the service used by the APM Server.
func NewService(as apmv1.ApmServer) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: as.Spec.HTTP.Service.ObjectMeta,
		Spec:       as.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = as.Namespace
	svc.ObjectMeta.Name = HTTPService(as.Name)

	labels := as.GetIdentityLabels()
	ports := []corev1.ServicePort{
		{
			Name:     as.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
