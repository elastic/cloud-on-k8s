// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"

	apmv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1alpha1"
	apmcerts "github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/config"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	apmname "github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates/http"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/pod"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
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
	c, err := add(mgr, reconciler)
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
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetRecorder(name),
		dynamicWatches: watches.NewDynamicWatches(),
		finalizers:     finalizer.NewHandler(client),
		Parameters:     params,
	}
}

func addWatches(c controller.Controller, r *ReconcileApmServer) error {
	// Watch for changes to ApmServer
	err := c.Watch(&source.Kind{Type: &apmv1alpha1.ApmServer{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch Deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1alpha1.ApmServer{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1alpha1.ApmServer{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &apmv1alpha1.ApmServer{},
	}); err != nil {
		return err
	}

	// dynamically watch referenced secrets to connect to Elasticsearch
	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, r.dynamicWatches.Secrets); err != nil {
		return err
	}

	return nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	return controller.New(name, mgr, controller.Options{Reconciler: r})
}

var _ reconcile.Reconciler = &ReconcileApmServer{}

// ReconcileApmServer reconciles an ApmServer object
type ReconcileApmServer struct {
	k8s.Client
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	dynamicWatches watches.DynamicWatches
	finalizers     finalizer.Handler
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

func (r *ReconcileApmServer) Scheme() *runtime.Scheme {
	return r.scheme
}

var _ driver.Interface = &ReconcileApmServer{}

// Reconcile reads that state of the cluster for a ApmServer object and makes changes based on the state read
// and what is in the ApmServer.Spec
func (r *ReconcileApmServer) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	defer common.LogReconciliationRun(log, request, &r.iteration)()

	var as apmv1alpha1.ApmServer
	if ok, err := association.FetchWithAssociation(r.Client, request, &as); !ok {
		return reconcile.Result{}, err
	}

	if common.IsPaused(as.ObjectMeta) {
		log.Info("Object is paused. Skipping reconciliation", "namespace", as.Namespace, "as_name", as.Name)
		return common.PauseRequeue, nil
	}

	if err := r.finalizers.Handle(&as, r.finalizersFor(as)...); err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Conflict while handling secret watch finalizer")
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}

	if as.IsMarkedForDeletion() {
		// APM server will be deleted nothing to do other than run finalizers
		return reconcile.Result{}, nil
	}

	if compatible, err := r.isCompatible(&as); err != nil || !compatible {
		return reconcile.Result{}, err
	}

	if err := annotation.UpdateControllerVersion(r.Client, &as, r.OperatorInfo.BuildInfo.Version); err != nil {
		return reconcile.Result{}, err
	}

	return r.doReconcile(request, &as)
}

func (r *ReconcileApmServer) isCompatible(as *apmv1alpha1.ApmServer) (bool, error) {
	selector := k8slabels.Set(map[string]string{labels.ApmServerNameLabelName: as.Name}).AsSelector()
	compat, err := annotation.ReconcileCompatibility(r.Client, as, selector, r.OperatorInfo.BuildInfo.Version)
	if err != nil {
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventCompatCheckError, "Error during compatibility check: %v", err)
	}
	return compat, err
}

func (r *ReconcileApmServer) doReconcile(request reconcile.Request, as *apmv1alpha1.ApmServer) (reconcile.Result, error) {
	state := NewState(request, as)
	svc, err := common.ReconcileService(r.Client, r.scheme, NewService(*as), as)
	if err != nil {
		return reconcile.Result{}, err
	}
	results := apmcerts.Reconcile(r, as, []corev1.Service{*svc}, r.CACertRotation)
	if results.HasError() {
		res, err := results.Aggregate()
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Certificate reconciliation error: %v", err)
		return res, err
	}

	state, err = r.reconcileApmServerDeployment(state, as)
	if err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return reconcile.Result{Requeue: true}, nil
		}
		k8s.EmitErrorEvent(r.recorder, err, as, events.EventReconciliationError, "Deployment reconciliation error: %v", err)
		return state.Result, err
	}

	state.UpdateApmServerExternalService(*svc)

	return r.updateStatus(state)
}

func (r *ReconcileApmServer) reconcileApmServerSecret(as *apmv1alpha1.ApmServer) (*corev1.Secret, error) {
	expectedApmServerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      apmname.SecretToken(as.Name),
			Labels:    labels.NewLabels(as.Name),
		},
		Data: map[string][]byte{
			SecretTokenKey: []byte(rand.String(24)),
		},
	}
	reconciledApmServerSecret := &corev1.Secret{}
	return reconciledApmServerSecret, reconciler.ReconcileResource(
		reconciler.Params{
			Client: r.Client,
			Scheme: r.scheme,

			Owner:      as,
			Expected:   expectedApmServerSecret,
			Reconciled: reconciledApmServerSecret,

			NeedsUpdate: func() bool {
				if !reflect.DeepEqual(reconciledApmServerSecret.Labels, expectedApmServerSecret.Labels) {
					return true
				}

				if reconciledApmServerSecret.Data == nil {
					return true
				}

				// re-use the secret token key if it exists
				existingSecretTokenKey, hasExistingSecretTokenKey := reconciledApmServerSecret.Data[SecretTokenKey]
				if hasExistingSecretTokenKey {
					expectedApmServerSecret.Data[SecretTokenKey] = existingSecretTokenKey
				}

				if !reflect.DeepEqual(reconciledApmServerSecret.Data, expectedApmServerSecret.Data) {
					return true
				}

				return false
			},
			UpdateReconciled: func() {
				reconciledApmServerSecret.Labels = expectedApmServerSecret.Labels
				reconciledApmServerSecret.Data = expectedApmServerSecret.Data
			},
			PreCreate: func() {
				log.Info("Creating apm server secret", "namespace", expectedApmServerSecret.Namespace, "secret_name", expectedApmServerSecret.Name, "as_name", as.Name)
			},
			PreUpdate: func() {
				log.Info("Updating apm server secret", "namespace", expectedApmServerSecret.Namespace, "secret_name", expectedApmServerSecret.Name, "as_name", as.Name)
			},
		},
	)
}

func (r *ReconcileApmServer) deploymentParams(
	as *apmv1alpha1.ApmServer,
	params PodSpecParams,
) (DeploymentParams, error) {

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
			return DeploymentParams{}, err
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
			Name:      certificates.HTTPCertsInternalSecretName(apmname.APMNamer, as.Name),
		}, &httpCerts)
		if err != nil {
			return DeploymentParams{}, err
		}
		if httpCert, ok := httpCerts.Data[certificates.CertFileName]; ok {
			_, _ = configChecksum.Write(httpCert)
		}
		httpCertsVolume := http.HTTPCertSecretVolume(apmname.APMNamer, as.Name)
		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, httpCertsVolume.Volume())
		apmServerContainer := pod.ContainerByName(podSpec.Spec, apmv1alpha1.APMServerContainerName)
		apmServerContainer.VolumeMounts = append(apmServerContainer.VolumeMounts, httpCertsVolume.VolumeMount())
	}

	podLabels[configChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))
	// TODO: also need to hash secret token?

	deploymentLabels := labels.NewLabels(as.Name)
	podSpec.Labels = defaults.SetDefaultLabels(podSpec.Labels, podLabels)

	return DeploymentParams{
		Name:            apmname.Deployment(as.Name),
		Namespace:       as.Namespace,
		Replicas:        as.Spec.NodeCount,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: podSpec,
	}, nil
}

func (r *ReconcileApmServer) reconcileApmServerDeployment(
	state State,
	as *apmv1alpha1.ApmServer,
) (State, error) {
	reconciledApmServerSecret, err := r.reconcileApmServerSecret(as)
	if err != nil {
		return state, err
	}
	reconciledConfigSecret, err := config.Reconcile(r.Client, r.scheme, as)
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

		ApmServerSecret: *reconciledApmServerSecret,
		ConfigSecret:    *reconciledConfigSecret,

		keystoreResources: keystoreResources,
	}
	params, err := r.deploymentParams(as, apmServerPodSpecParams)
	if err != nil {
		return state, err
	}

	deploy := NewDeployment(params)
	result, err := r.ReconcileDeployment(deploy, as)
	if err != nil {
		return state, err
	}
	state.UpdateApmServerState(result, *reconciledApmServerSecret)
	return state, nil
}

func (r *ReconcileApmServer) updateStatus(state State) (reconcile.Result, error) {
	current := state.originalApmServer
	if reflect.DeepEqual(current.Status, state.ApmServer.Status) {
		return state.Result, nil
	}
	if state.ApmServer.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Apm Server health degraded")
	}
	log.Info("Updating status", "namespace", state.ApmServer.Namespace, "as_name", state.ApmServer.Name, "iteration", atomic.LoadUint64(&r.iteration))
	err := r.Status().Update(state.ApmServer)
	if err != nil && errors.IsConflict(err) {
		log.V(1).Info("Conflict while updating status")
		return reconcile.Result{Requeue: true}, nil
	}

	return state.Result, err
}

// finalizersFor returns the list of finalizers applying to a given APM deployment
func (r *ReconcileApmServer) finalizersFor(as apmv1alpha1.ApmServer) []finalizer.Finalizer {
	return []finalizer.Finalizer{
		keystore.Finalizer(k8s.ExtractNamespacedName(&as), r.dynamicWatches, as.Kind()),
	}
}
