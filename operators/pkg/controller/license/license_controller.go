// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	match "github.com/elastic/k8s-operators/operators/pkg/controller/common/license"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/reconciler"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/license"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// defaultSafetyMargin is the duration used by this controller to ensure licenses are updated well before expiry
// In case of any operational issues affecting this controller clusters will have enough runway on their current license.
const defaultSafetyMargin = 30 * 24 * time.Hour

var (
	log = logf.Log.WithName("license-controller")
)

// clusterOnTrialError represents an error condition where reconciliation is aborted because a cluster is running
// on trial by explicit user request.
type clusterOnTrialError struct {
	nsn types.NamespacedName
}

func newClusterOnTrialError(name types.NamespacedName) *clusterOnTrialError {
	return &clusterOnTrialError{nsn: name}
}

func (c clusterOnTrialError) Error() string {
	return fmt.Sprintf("cluster %v is explicitly on trial, no reconciliation needed", c.nsn)
}

var _ error = clusterOnTrialError{}

// Reconcile reads the cluster license for the cluster being reconciled. If found, it checks whether it is still valid.
// If there is none it assigns a new one.
// In any case it schedules a new reconcile request to be processed when the license is about to expire.
// This happens independently from any watch triggered reconcile request.
func (r *ReconcileLicenses) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "request", request)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()
	result, err := r.reconcileInternal(request)
	if result.Requeue {
		log.Info("Re-queuing new license check immediately (rate-limited)", "cluster", request.NamespacedName)
	}
	if result.RequeueAfter > 0 {
		log.Info("Re-queuing new license check", "cluster", request.NamespacedName, "RequeueAfter", result.RequeueAfter)
	}
	return result, err
}

// Add creates a new EnterpriseLicense Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ operator.Parameters) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	c := k8s.WrapClient(mgr.GetClient())
	return &ReconcileLicenses{Client: c, scheme: mgr.GetScheme()}
}

func nextReconcile(expiry time.Time, safety time.Duration) reconcile.Result {
	return nextReconcileRelativeTo(time.Now(), expiry, safety)
}

func nextReconcileRelativeTo(now, expiry time.Time, safety time.Duration) reconcile.Result {
	requeueAfter := expiry.Add(-1 * (safety / 2)).Sub(now)
	if requeueAfter <= 0 {
		return reconcile.Result{Requeue: true}
	}
	return reconcile.Result{
		// requeue at expiry minus safetyMargin/2 to ensure we actually reissue a license on the next attempt
		RequeueAfter: requeueAfter,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("license-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ElasticsearchClusters
	if err := c.Watch(
		&source.Kind{Type: &v1alpha1.ElasticsearchCluster{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &v1alpha1.EnterpriseLicense{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(object handler.MapObject) []reconcile.Request {
			requests, err := listAffectedLicenses(
				k8s.WrapClient(mgr.GetClient()), k8s.ExtractNamespacedName(object.Meta),
			)
			if err != nil {
				// dropping the event(s) at this point
				log.Error(err, "failed to list affected clusters in enterprise license watch")
			}
			return requests
		}),
	}); err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileLicenses{}

// ReconcileLicenses reconciles EnterpriseLicenses with existing Elasticsearch clusters and creates ClusterLicenses for them.
type ReconcileLicenses struct {
	k8s.Client
	scheme *runtime.Scheme
	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// findLicenseFor tries to find a matching license for the given cluster identified by its namespaced name.
func findLicenseFor(c k8s.Client, clusterName types.NamespacedName) (v1alpha1.ClusterLicenseSpec, metav1.ObjectMeta, error) {
	var noLicense v1alpha1.ClusterLicenseSpec
	var noParent metav1.ObjectMeta
	var cluster v1alpha1.ElasticsearchCluster
	err := c.Get(clusterName, &cluster)
	if err != nil {
		return noLicense, noParent, err
	}
	desiredType := v1alpha1.LicenseTypeFromString(cluster.Labels[license.Expectation])
	if desiredType == v1alpha1.LicenseTypeTrial {
		return noLicense, noParent, newClusterOnTrialError(clusterName)
	}
	licenseList := v1alpha1.EnterpriseLicenseList{}
	err = c.List(&client.ListOptions{}, &licenseList)
	if err != nil {
		return noLicense, noParent, err
	}
	return match.BestMatch(licenseList.Items, desiredType)
}

// reconcileSecret upserts a secret in the namespace of the Elasticsearch cluster containing the signature of its license.
func reconcileSecret(
	c k8s.Client,
	cluster v1alpha1.ElasticsearchCluster,
	ref corev1.SecretKeySelector,
	ns string,
) (corev1.SecretKeySelector, error) {
	secretName := cluster.Name + "-license"
	secretKey := "sig"
	selector := corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: secretName,
		},
		Key: secretKey,
	}

	// fetch the user created secret from the controllers (global) namespace
	var globalSecret corev1.Secret
	err := c.Get(types.NamespacedName{Namespace: ns, Name: ref.Name}, &globalSecret)
	if err != nil {
		return selector, err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			secretKey: globalSecret.Data[ref.Key],
		},
	}
	// create/update a secret in the cluster's namespace containing the same data
	var reconciled corev1.Secret
	err = reconciler.ReconcileResource(reconciler.Params{
		Client:     c,
		Scheme:     scheme.Scheme,
		Owner:      &cluster,
		Expected:   &expected,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reflect.DeepEqual(reconciled.Data, expected.Data)
		},
		UpdateReconciled: func() {
			reconciled.Data = expected.Data
		},
	})
	return selector, err
}

// reconcileClusterLicense upserts a cluster license in the namespace of the given Elasticsearch cluster.
func (r *ReconcileLicenses) reconcileClusterLicense(
	cluster v1alpha1.ElasticsearchCluster,
	margin time.Duration,
) (time.Time, error) {
	var noResult time.Time
	clusterName := k8s.ExtractNamespacedName(&cluster)
	matchingSpec, parent, err := findLicenseFor(r, clusterName)
	if err != nil {
		return noResult, err
	}
	// make sure the signature secret is created in the cluster's namespace
	selector, err := reconcileSecret(r, cluster, matchingSpec.SignatureRef, parent.Namespace)
	if err != nil {
		return noResult, err
	}
	// reconcile the corresponding ClusterLicense also in the cluster's namespace
	toAssign := &v1alpha1.ClusterLicense{
		ObjectMeta: k8s.ToObjectMeta(clusterName), // use the cluster name as license name
		Spec:       matchingSpec,
	}
	toAssign.Labels = map[string]string{EnterpriseLicenseLabelName: parent.Name}
	toAssign.Spec.SignatureRef = selector
	var reconciled v1alpha1.ClusterLicense
	err = reconciler.ReconcileResource(reconciler.Params{
		Client:     r,
		Scheme:     r.scheme,
		Owner:      &cluster,
		Expected:   toAssign,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reconciled.IsValid(time.Now().Add(margin))
		},
		UpdateReconciled: func() {
			reconciled.Spec = toAssign.Spec
		},
		OnCreate: func() {
			log.Info("Assigning license", "cluster", clusterName, "license", matchingSpec.UID, "expiry", matchingSpec.ExpiryDate())
		},
		OnUpdate: func() {
			log.Info("Updating license to", "cluster", clusterName, "license", matchingSpec.UID, "expiry", matchingSpec.ExpiryDate())
		},
	})
	return matchingSpec.ExpiryDate(), err
}

func (r *ReconcileLicenses) reconcileInternal(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cluster to ensure it still exists
	owner := v1alpha1.ElasticsearchCluster{}
	err := r.Get(request.NamespacedName, &owner)
	if err != nil {
		if errors.IsNotFound(err) {
			// nothing to do no cluster
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !owner.DeletionTimestamp.IsZero() {
		// cluster is being deleted nothing to do
		return reconcile.Result{}, nil
	}
	safetyMargin := defaultSafetyMargin
	newExpiry, err := r.reconcileClusterLicense(owner, safetyMargin)
	if err != nil {
		switch err.(type) {
		case clusterOnTrialError:
			log.Info(err.Error()) // non treated as an error here, no license management for trials required
			return reconcile.Result{}, nil
		default:
			return reconcile.Result{Requeue: true}, err
		}
	}
	return nextReconcile(newExpiry, safetyMargin), nil
}
