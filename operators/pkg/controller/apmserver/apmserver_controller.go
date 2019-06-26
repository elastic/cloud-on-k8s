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
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
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

var log = logf.Log.WithName(name)

// Add creates a new ApmServer Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, params operator.Parameters) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileApmServer{
		Client:   k8s.WrapClient(mgr.GetClient()),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder(name),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ApmServer
	err = c.Watch(&source.Kind{Type: &apmv1alpha1.ApmServer{}}, &handler.EnqueueRequestForObject{})
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

	return nil
}

var _ reconcile.Reconciler = &ReconcileApmServer{}

// ReconcileApmServer reconciles an ApmServer object
type ReconcileApmServer struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

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

	if common.IsPaused(as.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	state := NewState(request, as)

	state, err = r.reconcileApmServerDeployment(state, as)
	if err != nil {
		return state.Result, err
	}

	svc := NewService(*as)
	res, err := common.ReconcileService(r.Client, r.scheme, svc, as)
	if err != nil {
		// TODO: consider updating some status here?
		return res, err
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

	// TODO: move server and config secrets into separate methods

	expectedApmServerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			// TODO: suffix+trim properly
			Name:   as.Name + "-apm-server",
			Labels: NewLabels(as.Name),
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

	cfg, err := config.NewConfigFromSpec(r.Client, *as)
	if err != nil {
		return state, err
	}

	cfgBytes, err := cfg.Render()
	if err != nil {
		return state, err
	}

	expectedConfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			// TODO: suffix+trim properly
			Name:   as.Name + "-config",
			Labels: NewLabels(as.Name),
		},
		Data: map[string][]byte{
			"apm-server.yml": cfgBytes,
		},
	}
	reconciledConfigSecret := &corev1.Secret{}
	if err := reconciler.ReconcileResource(
		reconciler.Params{
			Client: r.Client,
			Scheme: r.scheme,

			Owner:      as,
			Expected:   expectedConfigSecret,
			Reconciled: reconciledConfigSecret,

			NeedsUpdate: func() bool {
				return true
			},
			UpdateReconciled: func() {
				reconciledConfigSecret.Labels = expectedConfigSecret.Labels
				reconciledConfigSecret.Data = expectedConfigSecret.Data
			},
			PreCreate: func() {
				log.Info("Creating config secret", "name", expectedConfigSecret.Name)
			},
			PreUpdate: func() {
				log.Info("Updating config secret", "name", expectedConfigSecret.Name)
			},
		},
	); err != nil {
		return state, err
	}

	apmServerPodSpecParams := PodSpecParams{
		Version:         as.Spec.Version,
		CustomImageName: as.Spec.Image,

		PodTemplate: as.Spec.PodTemplate,

		ApmServerSecret: *reconciledApmServerSecret,
		ConfigSecret:    *reconciledConfigSecret,
	}

	podSpec := NewPodSpec(apmServerPodSpecParams)

	podLabels := NewLabels(as.Name)
	// add the config file checksum to the pod labels so a change triggers a rolling update
	podLabels[configChecksumLabelName] = fmt.Sprintf("%x", sha256.Sum224(cfgBytes))

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

	deploymentLabels := NewLabels(as.Name)
	podSpec.Labels = defaults.SetDefaultLabels(podSpec.Labels, podLabels)

	deploy := NewDeployment(DeploymentParams{
		// TODO: revisit naming?
		Name:            PseudoNamespacedResourceName(*as),
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
	return state.Result, r.Status().Update(state.ApmServer)
}
