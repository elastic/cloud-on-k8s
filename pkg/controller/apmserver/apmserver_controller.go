// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"

	"go.elastic.co/apm"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/config"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	apmname "github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	name                    = "apmserver-controller"
	esCAChecksumLabelName   = "apm.k8s.elastic.co/es-ca-file-checksum"
	configChecksumLabelName = "apm.k8s.elastic.co/config-file-checksum"

	// ApmBaseDir is the base directory of the APM server
	ApmBaseDir = "/usr/share/apm-server"
)

var (
	log = logf.Log.WithName(name)

	// ApmServerBin is the apm server binary file
	ApmServerBin = filepath.Join(ApmBaseDir, "apm-server")

	initContainerParameters = keystore.InitContainerParameters{
		KeystoreCreateCommand:         ApmServerBin + " keystore create --force",
		KeystoreAddCommand:            ApmServerBin + ` keystore add "$key" --stdin < "$filename"`,
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		DataVolumePath:                DataVolumePath,
	}
)

// Add creates a new ApmServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr, params)
	c, err := common.NewController(mgr, name, reconciler, params)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileApmServer {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileApmServer{
		Client:         client,
		recorder:       mgr.GetEventRecorderFor(name),
		dynamicWatches: watches.NewDynamicWatches(),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileApmServer) error {
	// Watch for changes to ApmServer
	err := c.Watch(&source.Kind{Type: &apmv1.ApmServer{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1.ApmServer{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1.ApmServer{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1.ApmServer{},
	}); err != nil {
		return err
	}

	// dynamically watch referenced secrets to connect to Elasticsearch
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileApmServer{}

// ReconcileApmServer reconciles an ApmServer object
type ReconcileApmServer struct {
	k8s.Client
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
}

func (r *ReconcileApmServer) K8sClient() k8s.Client {
	return r.Client
}

func (r *ReconcileApmServer) DynamicWatches() watches.DynamicWatches {
	return r.dynamicWatches
}

func (r *ReconcileApmServer) Recorder() record.EventRecorder {
	return r.recorder
}

var _ driver.Interface = &ReconcileApmServer{}

// Reconcile reads that state of the cluster for a ApmServer object and makes changes based on the state read
// and what is in the ApmServer.Spec
func (r *ReconcileApmServer) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, "as_name", &r.iteration)()
	tx, ctx := tracing.NewTransaction(r.Tracer, request.NamespacedName, "apmserver")
	defer tracing.EndTransaction(tx)

	var as apmv1.ApmServer
	if err := association.FetchWithAssociation(ctx, r.Client, request, &as); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(types.NamespacedName{
				Namespace: request.Namespace,
				Name:      request.Name,
			})
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if common.IsUnmanaged(as.ObjectMeta) {
		log.Info("Object currently not managed by this controller. Skipping reconciliation", "namespace", as.Namespace, "as_name", as.Name)
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(ctx, &as); err != nil || !compatible {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	// Remove any previous finalizer used in ECK v1.0.0-beta1 that we don't need anymore
	if err := finalizer.RemoveAll(r.Client, &as); err != nil {
		return reconcile.Result{}, err
	}

	if as.IsMarkedForDeletion() {
		// APM server will be deleted, clean up resources
		r.onDelete(k8s.ExtractNamespacedName(&as))
		return reconcile.Result{}, nil
	}

	if err := annotation.UpdateControllerVersion(ctx, r.Client, &as, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, tracing.CaptureError(ctx, err)
	}

	if !association.IsConfiguredIfSet(&as, r.recorder) {
		return reconcile.Result{}, nil
	}

	return r.doReconcile(ctx, request, &as)
}

func (r *ReconcileApmServer) isCompatible(ctx context.Context, as *apmv1.ApmServer) (bool, error) {
	selector := map[string]string{labels.ApmServerNameLabelName: as.Name}
	compat, err := annotation.ReconcileCompatibility(ctx, r.Client, as, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileApmServer) doReconcile(ctx context.Context, request reconcile.Request, as *apmv1.ApmServer) (reconcile.Result, error) {
	// Run validation in case the webhook is disabled
	if err := r.validate(ctx, as); err != nil {
		return reconcile.Result{}, err
	}

	state := NewState(request, as)
	svc, err := common.ReconcileService(ctx, r.Client, NewService(*as), as)
	if err != nil {
		return reconcile.Result{}, err
	}

	_, results := certificates.Reconciler{
		K8sClient:             r.K8sClient(),
		DynamicWatches:        r.DynamicWatches(),
		Object:                as,
		TLSOptions:            as.Spec.HTTP.TLS,
		Namer:                 apmname.APMNamer,
		Labels:                labels.NewLabels(as.Name),
		Services:              []corev1.Service{*svc},
		CACertRotation:        r.CACertRotation,
		CertRotation:          r.CertRotation,
		GarbageCollectSecrets: true,
	}.ReconcileCAAndHTTPCerts(ctx)
	if results.HasError() {
		res, err := results.Aggregate()
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return res, err
	}

	state, err = r.reconcileApmServerDeployment(ctx, state, as)
	if err != nil {
		if apierrors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return reconcile.Result{Requeue: true}, nil
		}
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Deployment reconciliation error: %v", err)
		return state.Result, tracing.CaptureError(ctx, err)
	}

	state.UpdateApmServerExternalService(*svc)

	// update status
	err = r.updateStatus(ctx, state)
	if err != nil && apierrors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status", "namespace", as.Namespace, "as", as.Name)
		return reconcile.Result{Requeue: true}, nil
	}
	res, err := results.WithError(err).Aggregate()
	k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Reconciliation error: %v", err)
	return res, err
}

func (r *ReconcileApmServer) validate(ctx context.Context, as *apmv1.ApmServer) error {
	span, vctx := apm.StartSpan(ctx, "validate", tracing.SpanTypeApp)
	defer span.End()

	if err := as.ValidateCreate(); err != nil {
		log.Error(err, "Validation failed")
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventReasonValidation, err.Error())
		return tracing.CaptureError(vctx, err)
	}

	return nil
}

func (r *ReconcileApmServer) onDelete(obj types.NamespacedName) {
	// Clean up watches set on secure settings
	r.dynamicWatches.Secrets.RemoveHandlerForKey(keystore.SecureSettingsWatchName(obj))
}

// reconcileApmServerToken reconciles a Secret containing the APM Server token.
// It reuses the existing token if possible.
func reconcileApmServerToken(c k8s.Client, as *apmv1.ApmServer) (corev1.Secret, error) {
	expectedApmServerSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      apmname.SecretToken(as.Name),
			Labels:    common.AddCredentialsLabel(labels.NewLabels(as.Name)),
		},
		Data: make(map[string][]byte),
	}
	// reuse the secret token if it already exists
	var existingSecret corev1.Secret
	err := c.Get(k8s.ExtractNamespacedName(&expectedApmServerSecret), &existingSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return corev1.Secret{}, err
	}
	if token, exists := existingSecret.Data[SecretTokenKey]; exists {
		expectedApmServerSecret.Data[SecretTokenKey] = token
	} else {
		expectedApmServerSecret.Data[SecretTokenKey] = common.RandomBytes(24)
	}

	return reconciler.ReconcileSecret(c, expectedApmServerSecret, as)
}

func (r *ReconcileApmServer) deploymentParams(
	as *apmv1.ApmServer,
	params PodSpecParams,
) (deployment.Params, error) {

	podSpec := newPodSpec(as, params)
	podLabels := labels.NewLabels(as.Name)

	// Build a checksum of the configuration, add it to the pod labels so a change triggers a rolling update
	configChecksum := sha256.New224()
	_, _ = configChecksum.Write(params.ConfigSecret.Data[config.ApmCfgSecretKey])
	if params.keystoreResources != nil {
		_, _ = configChecksum.Write([]byte(params.keystoreResources.Version))
	}

	if as.AssociationConf().CAIsConfigured() {
		esCASecretName := as.AssociationConf().GetCASecretName()
		// TODO: use apmServerCa to generate cert for deployment

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCAVolume := volume.NewSecretVolumeWithMountPath(
			esCASecretName,
			"elasticsearch-certs",
			filepath.Join(ApmBaseDir, config.CertificatesDir),
		)

		// build a checksum of the cert file used by ES, which we can use to cause the Deployment to roll the Apm Server
		// instances in the deployment when the ca file contents change. this is done because Apm Server do not support
		// updating the CA file contents without restarting the process.
		certsChecksum := ""
		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: as.Namespace, Name: esCASecretName}
		if err := r.Get(key, &esPublicCASecret); err != nil {
			return deployment.Params{}, err
		}
		if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
			certsChecksum = fmt.Sprintf("%x", sha256.Sum224(certPem))
		}
		// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
		// changes, which will trigger a rolling update)
		podLabels[esCAChecksumLabelName] = certsChecksum

		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, esCAVolume.Volume())

		for i := range podSpec.Spec.InitContainers {
			podSpec.Spec.InitContainers[i].VolumeMounts = append(podSpec.Spec.InitContainers[i].VolumeMounts, esCAVolume.VolumeMount())
		}

		for i := range podSpec.Spec.Containers {
			podSpec.Spec.Containers[i].VolumeMounts = append(podSpec.Spec.Containers[i].VolumeMounts, esCAVolume.VolumeMount())
		}
	}

	if as.Spec.HTTP.TLS.Enabled() {
		// fetch the secret to calculate the checksum
		var httpCerts corev1.Secret
		err := r.Get(types.NamespacedName{
			Namespace: as.Namespace,
			Name:      certificates.InternalCertsSecretName(apmname.APMNamer, as.Name),
		}, &httpCerts)
		if err != nil {
			return deployment.Params{}, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configChecksum.Write(httpCert)
		}
		httpCertsVolume := certificates.HTTPCertSecretVolume(apmname.APMNamer, as.Name)
		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, httpCertsVolume.Volume())
		apmServerContainer := pod.ContainerByName(podSpec.Spec, apmv1.ApmServerContainerName)
		apmServerContainer.VolumeMounts = append(apmServerContainer.VolumeMounts, httpCertsVolume.VolumeMount())
	}

	podLabels[configChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	// TODO: also need to hash secret token?

	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            apmname.Deployment(as.Name),
		Namespace:       as.Namespace,
		Replicas:        as.Spec.Count,
		Selector:        labels.NewLabels(as.Name),
		Labels:          labels.NewLabels(as.Name),
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.RollingUpdateDeploymentStrategyType,
	}, nil
}

func (r *ReconcileApmServer) reconcileApmServerDeployment(
	ctx context.Context,
	state State,
	as *apmv1.ApmServer,
) (State, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	tokenSecret, err := reconcileApmServerToken(r.Client, as)
	if err != nil {
		return state, err
	}
	reconciledConfigSecret, err := config.Reconcile(r.Client, as)
	if err != nil {
		return state, err
	}

	keystoreResources, err := keystore.NewResources(
		r,
		as,
		apmname.APMNamer,
		labels.NewLabels(as.Name),
		initContainerParameters,
	)
	if err != nil {
		return state, err
	}

	apmServerPodSpecParams := PodSpecParams{
		Version:         as.Spec.Version,
		CustomImageName: as.Spec.Image,

		PodTemplate: as.Spec.PodTemplate,

		TokenSecret:  tokenSecret,
		ConfigSecret: reconciledConfigSecret,

		keystoreResources: keystoreResources,
	}
	params, err := r.deploymentParams(as, apmServerPodSpecParams)
	if err != nil {
		return state, err
	}

	deploy := deployment.New(params)
	result, err := deployment.Reconcile(r.K8sClient(), deploy, as)
	if err != nil {
		return state, err
	}
	state.UpdateApmServerState(result, tokenSecret)
	return state, nil
}

func (r *ReconcileApmServer) updateStatus(ctx context.Context, state State) error {
	span, _ := apm.StartSpan(ctx, "update_status", tracing.SpanTypeApp)
	defer span.End()

	current := state.originalApmServer
	if reflect.DeepEqual(current.Status, state.ApmServer.Status) {
		return nil
	}
	if state.ApmServer.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Apm Server health degraded")
	}
	log.V(1).Info("Updating status",
		"iteration", atomic.LoadUint64(&r.iteration),
		"namespace", state.ApmServer.Namespace,
		"as_name", state.ApmServer.Name,
		"status", state.ApmServer.Status,
	)
	return common.UpdateStatus(r.Client, state.ApmServer)
}
