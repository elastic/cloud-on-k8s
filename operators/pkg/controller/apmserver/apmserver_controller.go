// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	apmv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/config"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/labels"
	apmname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/association/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

var (
	log = logf.Log.WithName(name)

	initContainerParameters = keystore.InitContainerParameters{
		KeystoreCreateCommand:         "/usr/share/apm-server/apm-server keystore create --force",
		KeystoreAddCommand:            "/usr/share/apm-server/apm-server keystore add",
		SecureSettingsVolumeMountPath: keystore.SecureSettingsVolumeMountPath,
		DataVolumePath:                DataVolumePath,
	}
)

// Add creates a new ApmServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	reconciler := newReconciler(mgr)
	c, err := add(mgr, reconciler)
	if err != nil {
		return err
	}
	return addWatches(c, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) *ReconcileApmServer {
	client := k8s.WrapClient(mgr.GetClient())
	return &ReconcileApmServer{
		Client:         client,
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetRecorder(name),
		dynamicWatches: watches.NewDynamicWatches(),
		finalizers:     finalizer.NewHandler(client),
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

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a ApmServer object and makes changes based on the state read
// and what is in the ApmServer.Spec
func (r *ReconcileApmServer) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the ApmServer resource
	as := &apmv1alpha1.ApmServer{}
	err := r.Get(request.NamespacedName, as)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if common.IsPaused(as.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	if err := r.finalizers.Handle(as, r.finalizersFor(*as)...); err != nil {
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

	state := NewState(request, as)

	state, err = r.reconcileApmServerDeployment(state, as)
	if err != nil {
		if errors.IsConflict(err) {
			log.V(1).Info("Conflict while updating status")
			return reconcile.Result{Requeue: true}, nil
		}
		return state.Result, err
	}

	svc := NewService(*as)
	_, err = common.ReconcileService(r.Client, r.scheme, svc, as)
	if err != nil {
		// TODO: consider updating some status here?
		return reconcile.Result{}, err
	}

	state.UpdateApmServerExternalService(*svc)

	return r.updateStatus(state)
}

func (r *ReconcileApmServer) reconcileApmServerDeployment(
	state State,
	as *apmv1alpha1.ApmServer,
) (State, error) {
	if !as.Spec.Output.Elasticsearch.IsConfigured() {
		log.Info("Aborting ApmServer deployment reconciliation as no Elasticsearch output is configured")
		return state, nil
	}

	// TODO: move server secret into separate method

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
	if err := reconciler.ReconcileResource(
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
				log.Info("Creating apm server secret", "name", expectedApmServerSecret.Name)
			},
			PreUpdate: func() {
				log.Info("Updating apm server secret", "name", expectedApmServerSecret.Name)
			},
		},
	); err != nil {
		return state, err
	}

	reconciledConfigSecret, err := config.Reconcile(r.Client, r.scheme, as)
	if err != nil {
		return state, err
	}

	keystoreResources, err := keystore.NewResources(
		r.Client,
		r.recorder,
		r.dynamicWatches,
		as,
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

	podSpec := newPodSpec(as, apmServerPodSpecParams)

	podLabels := labels.NewLabels(as.Name)

	// Build a checksum of the configuration, add it to the pod labels so a change triggers a rolling update
	configChecksum := sha256.New224()
	configChecksum.Write(reconciledConfigSecret.Data[config.ApmCfgSecretKey])
	if keystoreResources != nil {
		configChecksum.Write([]byte(keystoreResources.Version))
	}
	podLabels[configChecksumLabelName] = fmt.Sprintf("%x", configChecksum.Sum(nil))

	esCASecretName := as.Spec.Output.Elasticsearch.SSL.CertificateAuthorities.SecretName
	if esCASecretName != "" {
		// TODO: use apmServerCa to generate cert for deployment

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCAVolume := volume.NewSecretVolumeWithMountPath(
			esCASecretName,
			"elasticsearch-certs",
			"/usr/share/apm-server/config/elasticsearch-certs",
		)

		// build a checksum of the cert file used by ES, which we can use to cause the Deployment to roll the Apm Server
		// instances in the deployment when the ca file contents change. this is done because Apm Server do not support
		// updating the CA file contents without restarting the process.
		certsChecksum := ""
		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: as.Namespace, Name: esCASecretName}
		if err := r.Get(key, &esPublicCASecret); err != nil {
			return state, err
		}
		if certPem, ok := esPublicCASecret.Data[certificates.CertFileName]; ok {
			certsChecksum = fmt.Sprintf("%x", sha256.Sum224(certPem))
		}
		// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
		// changes, which will trigger a rolling update)
		podLabels[esCAChecksumLabelName] = certsChecksum

		podSpec.Spec.Volumes = append(podSpec.Spec.Volumes, esCAVolume.Volume())

		for i, container := range podSpec.Spec.InitContainers {
			podSpec.Spec.InitContainers[i].VolumeMounts = append(container.VolumeMounts, esCAVolume.VolumeMount())
		}

		for i, container := range podSpec.Spec.Containers {
			podSpec.Spec.Containers[i].VolumeMounts = append(container.VolumeMounts, esCAVolume.VolumeMount())
		}
	}

	// TODO: also need to hash secret token?

	deploymentLabels := labels.NewLabels(as.Name)
	podSpec.Labels = defaults.SetDefaultLabels(podSpec.Labels, podLabels)

	deploy := NewDeployment(DeploymentParams{
		Name:            apmname.Deployment(as.Name),
		Namespace:       as.Namespace,
		Replicas:        as.Spec.NodeCount,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: podSpec,
	})
	result, err := r.ReconcileDeployment(deploy, as)
	if err != nil {
		return state, err
	}
	state.UpdateApmServerState(result, *expectedApmServerSecret)
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
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))

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
		keystore.Finalizer(k8s.ExtractNamespacedName(&as), r.dynamicWatches, &as),
	}
}
