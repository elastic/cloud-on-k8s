// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/reconciler"
	esclient "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/client"
	esname "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	name = "license-controller"

	// defaultSafetyMargin is the duration used by this controller to ensure licenses are updated well before expiry
	// In case of any operational issues affecting this controller clusters will have enough runway on their current license.
	defaultSafetyMargin  = 30 * 24 * time.Hour
	minimumRetryInterval = 1 * time.Hour
)

var log = logf.Log.WithName(name)

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
func Add(mgr manager.Manager, p operator.Parameters) error {
	return add(mgr, newReconciler(mgr, p.OperatorNamespace))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, ns string) reconcile.Reconciler {
	c := k8s.WrapClient(mgr.GetClient())
	return &ReconcileLicenses{
		Client:  c,
		scheme:  mgr.GetScheme(),
		checker: license.NewLicenseChecker(c, ns),
	}
}

func nextReconcile(expiry time.Time, safety time.Duration) reconcile.Result {
	return nextReconcileRelativeTo(time.Now(), expiry, safety)
}

func nextReconcileRelativeTo(now, expiry time.Time, safety time.Duration) reconcile.Result {
	// short-circuit to default if no expiry given
	if expiry.IsZero() {
		return reconcile.Result{
			RequeueAfter: minimumRetryInterval,
		}
	}
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
	c, err := controller.New(name, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Elasticsearch clusters.
	if err := c.Watch(
		&source.Kind{Type: &v1alpha1.Elasticsearch{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(object handler.MapObject) []reconcile.Request {
			secret, ok := object.Object.(*corev1.Secret)
			if !ok {
				log.Error(
					fmt.Errorf("unexpected object type %T in watch handler, expected Secret", object.Object),
					"dropping watch event due to error in handler")
				return nil
			}
			if !license.IsEnterpriseLicense(*secret) {
				return nil
			}
			// TODO reconcile license secrets and augment license secret metadata with license UID to minimize parsing
			license, err := license.ParseEnterpriseLicense(secret.Data)
			if err != nil {
				log.Error(err, "ignoring invalid or unparseable license in watch handler")
				return nil
			}

			rs, err := listAffectedLicenses(
				k8s.WrapClient(mgr.GetClient()), license.License.UID,
			)
			if err != nil {
				// dropping the event(s) at this point
				log.Error(err, "failed to list affected clusters in enterprise license watch")
				return nil
			}
			return rs
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
	checker   license.Checker
}

// findLicense tries to find the best license available.
func findLicense(c k8s.Client, checker license.Checker) (esclient.License, string, bool, error) {
	licenseList, errs := license.EnterpriseLicensesOrErrors(c)
	if len(errs) > 0 {
		log.Info("Ignoring invalid license objects", "errors", errs)
	}
	return license.BestMatch(licenseList, checker.Valid)
}

// reconcileSecret upserts a secret in the namespace of the Elasticsearch cluster containing the signature of its license.
func reconcileSecret(
	c k8s.Client,
	cluster v1alpha1.Elasticsearch,
	parent string,
	esLicense esclient.License,
) error {
	secretName := esname.LicenseSecretName(cluster.Name)

	licenseBytes, err := json.Marshal(esLicense)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				license.LicenseLabelName: parent,
				common.TypeLabelName:     string(license.LicenseLabelElasticsearch),
			},
		},
		Data: map[string][]byte{
			license.FileName: licenseBytes,
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
	return err
}

// reconcileClusterLicense upserts a cluster license in the namespace of the given Elasticsearch cluster.
func (r *ReconcileLicenses) reconcileClusterLicense(
	cluster v1alpha1.Elasticsearch,
	margin time.Duration,
) (time.Time, error) {
	var noResult time.Time
	matchingSpec, parent, found, err := findLicense(r, r.checker)
	if err != nil {
		return noResult, err
	}
	if !found {
		// no license, nothing to do
		return noResult, nil
	}
	// make sure the signature secret is created in the cluster's namespace
	if err = reconcileSecret(r, cluster, parent, matchingSpec); err != nil {
		return noResult, err
	}
	return matchingSpec.ExpiryTime(), err
}

func (r *ReconcileLicenses) reconcileInternal(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cluster to ensure it still exists
	cluster := v1alpha1.Elasticsearch{}
	err := r.Get(request.NamespacedName, &cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			// nothing to do no cluster
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if !cluster.DeletionTimestamp.IsZero() {
		// cluster is being deleted nothing to do
		return reconcile.Result{}, nil
	}

	safetyMargin := defaultSafetyMargin
	newExpiry, err := r.reconcileClusterLicense(cluster, safetyMargin)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}
	return nextReconcile(newExpiry, safetyMargin), nil
}
