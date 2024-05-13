// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	eslabel "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

const (
	name = "license-controller"

	// defaultSafetyMargin is the duration used by this controller to ensure licenses are updated well before expiry
	// In case of any operational issues affecting this controller clusters will have enough runway on their current license.
	defaultSafetyMargin  = 30 * 24 * time.Hour
	minimumRetryInterval = 1 * time.Hour
)

// Reconcile reads the cluster license for the cluster being reconciled. If found, it checks whether it is still valid.
// If there is none it assigns a new one.
// In any case it schedules a new reconcile request to be processed when the license is about to expire.
// This happens independently from any watch triggered reconcile request.
func (r *ReconcileLicenses) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	ctx = common.NewReconciliationContext(ctx, &r.iteration, r.Tracer, name, "es_name", request)
	defer common.LogReconciliationRun(ulog.FromContext(ctx))()
	defer tracing.EndContextTransaction(ctx)

	results := r.reconcileInternal(ctx, request)
	current, err := results.Aggregate()
	ulog.FromContext(ctx).V(1).Info("Reconcile result", "requeue", current.Requeue, "requeueAfter", current.RequeueAfter)
	return current, err
}

// Add creates a new EnterpriseLicense Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, p operator.Parameters) error {
	r := newReconciler(mgr, p)
	c, err := common.NewController(mgr, name, r, p)
	if err != nil {
		return err
	}
	return addWatches(mgr, c, r.Client)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, params operator.Parameters) *ReconcileLicenses {
	c := mgr.GetClient()
	return &ReconcileLicenses{
		Client:     c,
		Parameters: params,
		checker:    license.NewLicenseChecker(c, params.OperatorNamespace),
		recorder:   mgr.GetEventRecorderFor(name),
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

// addWatches adds a new Controller to mgr with r as the reconcile.Reconciler
func addWatches(mgr manager.Manager, c controller.Controller, k8sClient k8s.Client) error {
	log := ulog.Log // no context available for contextual logging
	// Watch for changes to Elasticsearch clusters.
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &esv1.Elasticsearch{}, &handler.TypedEnqueueRequestForObject[*esv1.Elasticsearch]{})); err != nil {
		return err
	}

	if err := c.Watch(source.Kind(mgr.GetCache(), &corev1.Secret{},
		handler.TypedEnqueueRequestsFromMapFunc[*corev1.Secret](func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
			if !license.IsOperatorLicense(*secret) {
				return nil
			}

			// if a license is added/modified we want to update for potentially all clusters managed by this instance
			// of ECK which is why we are listing all Elasticsearch clusters here and trigger a reconciliation
			rs, err := reconcileRequestsForAllClusters(k8sClient, log)
			if err != nil {
				// dropping the event(s) at this point
				log.Error(err, "failed to list affected clusters in enterprise license watch")
				return nil
			}
			return rs
		}),
	)); err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileLicenses{}

// ReconcileLicenses reconciles EnterpriseLicenses with existing Elasticsearch clusters and creates ClusterLicenses for them.
type ReconcileLicenses struct {
	k8s.Client
	operator.Parameters
	// iteration is the number of times this controller has run its Reconcile method
	iteration uint64
	checker   license.Checker
	recorder  record.EventRecorder
}

// findLicense tries to find the best Elastic stack license available.
func (r *ReconcileLicenses) findLicense(ctx context.Context, c k8s.Client, checker license.Checker, minVersion *version.Version) (esclient.License, string, bool) {
	licenseList, errs := license.EnterpriseLicensesOrErrors(c)
	if len(errs) > 0 {
		ulog.FromContext(ctx).Error(utilerrors.NewAggregate(errs), "Ignoring invalid license objects")
		recordInvalidLicenseEvents(errs, r.recorder)
	}
	valid := func(l license.EnterpriseLicense) (bool, error) {
		return checker.Valid(ctx, l)
	}
	return license.BestMatch(ctx, minVersion, licenseList, valid)
}

func recordInvalidLicenseEvents(errs []error, recorder record.EventRecorder) {
	for _, err := range errs {
		var licenseErr *license.Error
		if errors.As(err, &licenseErr) {
			recorder.Event(licenseErr.Source, corev1.EventTypeWarning, events.EventReasonInvalidLicense, err.Error())
		}
	}
}

// reconcileSecret upserts a secret in the namespace of the Elasticsearch cluster containing the signature of its license.
func reconcileSecret(
	ctx context.Context,
	c k8s.Client,
	cluster esv1.Elasticsearch,
	parent string,
	esLicense esclient.License,
) error {
	secretName := esv1.LicenseSecretName(cluster.Name)

	licenseBytes, err := json.Marshal(esLicense)
	if err != nil {
		return err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				commonv1.TypeLabelName:    license.Type,
				license.LicenseLabelName:  parent,
				license.LicenseLabelScope: string(license.LicenseScopeElasticsearch),
				license.LicenseLabelType:  esLicense.Type,
			},
		},
		Data: map[string][]byte{
			license.FileName: licenseBytes,
		},
	}
	// create/update a secret in the cluster's namespace containing the same data
	_, err = reconciler.ReconcileSecret(ctx, c, expected, &cluster)
	return err
}

// reconcileClusterLicense upserts a cluster license in the namespace of the given Elasticsearch cluster.
// Returns time to next reconciliation, bool whether a license is configured at all and optional error.
func (r *ReconcileLicenses) reconcileClusterLicense(ctx context.Context, cluster esv1.Elasticsearch) (time.Time, bool, error) {
	log := ulog.FromContext(ctx)

	var noResult time.Time
	minVersion, err := r.minVersion(cluster)
	if err != nil {
		return noResult, true, err
	}
	matchingSpec, parent, found := r.findLicense(ctx, r, r.checker, minVersion)
	if !found {
		// no matching license found, delete cluster level license if it exists to revert to basic
		clusterLicenseNSN := types.NamespacedName{Namespace: cluster.Namespace, Name: esv1.LicenseSecretName(cluster.Name)}
		log.V(1).Info("No enterprise license found. Attempting to remove cluster license secret", "namespace", cluster.Namespace, "es_name", cluster.Name)
		err := k8s.DeleteSecretIfExists(ctx, r.Client, clusterLicenseNSN)
		return noResult, false, err
	}
	log.V(1).Info("Found license for cluster", "eck_license", parent, "es_license", matchingSpec.UID, "license_type", matchingSpec.Type, "namespace", cluster.Namespace, "es_name", cluster.Name)
	// make sure the signature secret is created in the cluster's namespace
	if err := reconcileSecret(ctx, r, cluster, parent, matchingSpec); err != nil {
		return noResult, false, err
	}
	return matchingSpec.ExpiryTime(), false, nil
}

func (r *ReconcileLicenses) minVersion(cluster esv1.Elasticsearch) (*version.Version, error) {
	pods, err := sset.GetActualPodsForCluster(r, cluster)
	if err != nil {
		return nil, err
	}
	minVersion, err := version.MinInPods(pods, eslabel.VersionLabelName)
	if err != nil {
		return nil, err
	}
	if minVersion == nil {
		v, err := version.Parse(cluster.Spec.Version)
		if err != nil {
			return nil, err
		}
		minVersion = &v
	}
	return minVersion, nil
}

func (r *ReconcileLicenses) reconcileInternal(ctx context.Context, request reconcile.Request) *reconciler.Results {
	res := &reconciler.Results{}

	// Fetch the cluster to ensure it still exists
	cluster := esv1.Elasticsearch{}
	err := r.Get(ctx, request.NamespacedName, &cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// nothing to do no cluster
			return res
		}
		return res.WithError(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		// cluster is being deleted nothing to do
		return res
	}

	newExpiry, noLicense, err := r.reconcileClusterLicense(ctx, cluster)
	if err != nil {
		return res.WithError(err)
	}
	margin := defaultSafetyMargin
	if noLicense {
		// don't apply safety margin if we don't have a license but use requested requeue time as specified in newExpiry
		margin = 0
	}
	return res.WithResult(nextReconcile(newExpiry, margin))
}
