// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	kibanav1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	log            = logf.Log.WithName("kibana-controller")
)

const (
	caChecksumLabelName = "kibana.k8s.elastic.co/ca-file-checksum"
)

// Add creates a new Kibana Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ operator.Parameters) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileKibana{
		Client:   k8s.WrapClient(mgr.GetClient()),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("kibana-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("kibana-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Kibana
	if err := c.Watch(&source.Kind{Type: &kibanav1alpha1.Kibana{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch deployments
	if err := c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kibanav1alpha1.Kibana{},
	}); err != nil {
		return err
	}

	// Watch services
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &kibanav1alpha1.Kibana{},
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileKibana{}

// ReconcileKibana reconciles a Kibana object
type ReconcileKibana struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Kibana object and makes changes based on the state read and what is
// in the Kibana.Spec
func (r *ReconcileKibana) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the Kibana instance
	kb := &kibanav1alpha1.Kibana{}
	err := r.Get(request.NamespacedName, kb)

	if common.IsPaused(kb.ObjectMeta) {
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

	state := NewState(request, kb)

	state, err = r.reconcileKibanaDeployment(state, kb)
	if err != nil {
		return state.Result, err
	}

	res, err := common.ReconcileService(r.Client, r.scheme, NewService(*kb), kb)
	if err != nil {
		// TODO: consider updating some status here?
		return res, err
	}

	return r.updateStatus(state)
}

func (r *ReconcileKibana) reconcileKibanaDeployment(
	state State,
	kb *kibanav1alpha1.Kibana,
) (State, error) {
	if !kb.Spec.Elasticsearch.IsConfigured() {
		log.Info("Aborting Kibana deployment reconciliation as no Elasticsearch backend is configured")
		return state, nil
	}
	var auth kibanav1alpha1.ElasticsearchInlineAuth
	if kb.Spec.Elasticsearch.Auth.Inline != nil {
		auth = *kb.Spec.Elasticsearch.Auth.Inline
	}
	kibanaPodSpecParams := PodSpecParams{
		Version:          kb.Spec.Version,
		CustomImageName:  kb.Spec.Image,
		ElasticsearchUrl: kb.Spec.Elasticsearch.URL,
		// TODO: handle different ways to provide auth credentials
		User: auth,
	}

	kibanaPodSpec := NewPodSpec(kibanaPodSpecParams)
	labels := NewLabels(kb.Name)
	podLabels := NewLabels(kb.Name)

	if kb.Spec.Elasticsearch.CaCertSecret != nil {
		// TODO: use kibanaCa to generate cert for deployment
		// to do that, EnsureNodeCertificateSecretExists needs a deployment variant.

		// TODO: this is a little ugly as it reaches into the ES controller bits
		esCertsVolume := volume.NewSecretVolumeWithMountPath(
			*kb.Spec.Elasticsearch.CaCertSecret,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		// build a checksum of the ca file used by ES, which we can use to cause the Deployment to roll the Kibana
		// instances in the deployment when the ca file contents change. this is done because Kibana do not support
		// updating the ca.pem file contents without restarting the process.
		caChecksum := ""
		var esPublicCASecret corev1.Secret
		key := types.NamespacedName{Namespace: kb.Namespace, Name: *kb.Spec.Elasticsearch.CaCertSecret}
		if err := r.Get(key, &esPublicCASecret); err != nil {
			return state, err
		}
		if capem, ok := esPublicCASecret.Data[nodecerts.SecretCAKey]; ok {
			caChecksum = fmt.Sprintf("%x", sha256.Sum224(capem))
		}
		// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
		// changes, which will trigger a rolling update)
		podLabels[caChecksumLabelName] = caChecksum

		kibanaPodSpec.Volumes = append(kibanaPodSpec.Volumes, esCertsVolume.Volume())

		for i, container := range kibanaPodSpec.InitContainers {
			kibanaPodSpec.InitContainers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())
		}

		for i, container := range kibanaPodSpec.Containers {
			kibanaPodSpec.Containers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())

			kibanaPodSpec.Containers[i].Env = append(
				kibanaPodSpec.Containers[i].Env,
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_CERTIFICATEAUTHORITIES",
					Value: strings.Join([]string{esCertsVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
				},
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_VERIFICATIONMODE",
					Value: "certificate",
				},
			)
		}
	}

	deploy := NewDeployment(DeploymentParams{
		// TODO: revisit naming?
		Name:      PseudoNamespacedResourceName(*kb),
		Namespace: kb.Namespace,
		Replicas:  kb.Spec.NodeCount,
		Selector:  labels,
		Labels:    labels,
		PodLabels: podLabels,
		PodSpec:   kibanaPodSpec,
	})
	result, err := r.ReconcileDeployment(deploy, kb)
	if err != nil {
		return state, err
	}
	state.UpdateKibanaState(result)
	return state, nil
}

func (r *ReconcileKibana) updateStatus(state State) (reconcile.Result, error) {
	current := state.originalKibana
	if reflect.DeepEqual(current.Status, state.Kibana.Status) {
		return state.Result, nil
	}
	if state.Kibana.Status.IsDegraded(current.Status) {
		r.recorder.Event(current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	return state.Result, r.Status().Update(state.Kibana)
}
