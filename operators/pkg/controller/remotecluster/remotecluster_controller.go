// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package remotecluster

import (
	"reflect"
	"sync/atomic"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/license"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/watches"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	EventReasonLocalCaCertNotFound = "LocalClusterCaNotFound"
	EventReasonRemoteCACertMissing = "RemoteClusterCaNotFound"
	CaCertMissingError             = "Cannot find CA certificate for %s cluster %s/%s"
	EventReasonConfigurationError  = "ConfigurationError"
	ClusterNameLabelMissing        = "label " + label.ClusterNameLabelName + " is missing"
)

var (
	log            = logf.Log.WithName("remotecluster-controller")
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 20 * time.Second}
)

// Add creates a new RemoteCluster Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, parameter operator.Parameters) error {
	r := newReconciler(mgr, parameter.OperatorNamespace)
	c, err := add(mgr, r)
	if err != nil {
		return err
	}
	return addWatches(c, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, operatorNs string) *ReconcileRemoteCluster {
	c := k8s.WrapClient(mgr.GetClient())
	return &ReconcileRemoteCluster{
		Client:         c,
		scheme:         mgr.GetScheme(),
		watches:        watches.NewDynamicWatches(),
		recorder:       mgr.GetRecorder("remotecluster-controller"),
		licenseChecker: license.NewLicenseChecker(c, operatorNs),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) (controller.Controller, error) {
	// Create a new controller
	c, err := controller.New("remotecluster-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return c, err
	}
	return c, nil
}

var _ reconcile.Reconciler = &ReconcileRemoteCluster{}

// ReconcileRemoteCluster reconciles a RemoteCluster object.
type ReconcileRemoteCluster struct {
	k8s.Client
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	watches        watches.DynamicWatches
	licenseChecker *license.Checker

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a RemoteCluster object and makes changes based on the state read
// and what is in the RemoteCluster.Spec
func (r *ReconcileRemoteCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the RemoteCluster instance
	instance := v1alpha1.RemoteCluster{}
	err := r.Get(request.NamespacedName, &instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// nothing to do
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if common.IsPaused(instance.ObjectMeta) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	enabled, err := r.licenseChecker.CommercialFeaturesEnabled()
	if err != nil {
		return defaultRequeue, err
	}
	if !enabled {
		log.Info(
			"Remote cluster controller is a commercial feature. Commercial features are disabled",
			"iteration", currentIteration,
		)
		r.silentUpdateStatus(instance, v1alpha1.RemoteClusterStatus{
			State: v1alpha1.RemoteClusterFeatureDisabled,
		})
		return reconcile.Result{}, nil
	}

	// Use the driver to create the remote cluster
	status, err := doReconcile(r, instance)
	if err != nil {
		// Driver reported an error, try to update the status as a best effort
		r.silentUpdateStatus(instance, status)
		return defaultRequeue, err
	}

	// status contains important information for the Elasticsearch controller, we must ensure that it is updated
	return reconcile.Result{}, r.updateStatus(instance, status)
}

// silentUpdateStatus updates the status as a best effort, it is used when the driver has already returned an error.
func (r *ReconcileRemoteCluster) silentUpdateStatus(
	instance v1alpha1.RemoteCluster,
	status v1alpha1.RemoteClusterStatus,
) {
	if err := r.updateStatus(instance, status); err != nil {
		log.Error(err, "Error while updating status")
	}
}

// updateStatus updates the status and returns any errors encountered.
func (r *ReconcileRemoteCluster) updateStatus(
	instance v1alpha1.RemoteCluster,
	status v1alpha1.RemoteClusterStatus,
) error {
	if reflect.DeepEqual(instance.Status, status) {
		return nil
	}
	instance.Status = status
	if err := r.Status().Update(&instance); err != nil {
		return err
	}
	return nil
}
