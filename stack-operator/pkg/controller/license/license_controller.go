package license

import (
	"context"
	"reflect"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	match "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/reconciler"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
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

var (
	log = logf.Log.WithName("license-controller")
)

// Add creates a new EnterpriseLicense Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ operator.Parameters) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileLicenses{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

func newContext() context.Context {
	return context.TODO()
}

func defaultSafetyMargin() v1alpha1.SafetyMargin {
	return v1alpha1.SafetyMargin{
		ValidSince: 2 * 24 * time.Hour,
		ValidFor:   30 * 24 * time.Hour,
	}
}

func nextReconcile(expiry time.Time, safety v1alpha1.SafetyMargin) reconcile.Result {
	return nextReconcileRelativeTo(time.Now(), expiry, safety)
}

func nextReconcileRelativeTo(now, expiry time.Time, safety v1alpha1.SafetyMargin) reconcile.Result {
	requeueAfter := expiry.Add(-1 * (safety.ValidFor / 2)).Sub(now)
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
	return nil
}

var _ reconcile.Reconciler = &ReconcileLicenses{}

// ReconcileLicenses reconciles EnterpriseLicenses with existing Elasticsearch clusters and creates ClusterLicenses for them.
type ReconcileLicenses struct {
	client.Client
	scheme *runtime.Scheme
}

// findLicenseFor tries to find a matching license for the given cluster identified by its namespaced name.
func findLicenseFor(c client.Client, clusterName types.NamespacedName) (v1alpha1.ClusterLicense, error) {
	var noLicense v1alpha1.ClusterLicense
	cluster := v1alpha1.ElasticsearchCluster{}
	err := c.Get(newContext(), clusterName, &cluster)
	if err != nil {
		return noLicense, err
	}
	var kind match.DesiredLicenseType
	s, ok := cluster.Labels[license.Expectation]
	if ok {
		kind = v1alpha1.LicenseTypeFromString(s)
	}

	licenseList := v1alpha1.EnterpriseLicenseList{}
	err = c.List(newContext(), &client.ListOptions{}, &licenseList)
	if err != nil {
		return noLicense, err
	}
	return match.BestMatch(licenseList.Items, kind)
}

// reconcileSecret upserts a secret in the namespace of the Elasticsearch cluster containing the signature of its license.
func reconcileSecret(c client.Client, cluster v1alpha1.ElasticsearchCluster, l v1alpha1.ClusterLicense) (corev1.SecretKeySelector, error) {
	secretName := cluster.Name + "-license"
	secretKey := "sig"
	selector := corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: secretName,
		},
		Key: secretKey,
	}

	var controllerSecret corev1.Secret
	err := c.Get(newContext(), types.NamespacedName{Namespace: l.Namespace, Name: l.Spec.SignatureRef.Name}, &controllerSecret)
	if err != nil {
		return selector, err
	}

	expected := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			secretKey: controllerSecret.Data[l.Spec.SignatureRef.Key],
		},
	}
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
	margin v1alpha1.SafetyMargin,
) (time.Time, error) {
	var noResult time.Time
	clusterName := k8s.ExtractNamespacedName(cluster.ObjectMeta)
	match, err := findLicenseFor(r, clusterName)
	if err != nil {
		return noResult, err
	}
	selector, err := reconcileSecret(r, cluster, match)
	if err != nil {
		return noResult, err
	}

	toAssign := match.DeepCopy()
	toAssign.ObjectMeta = k8s.ToObjectMeta(clusterName)
	toAssign.Spec.SignatureRef = selector
	var reconciled v1alpha1.ClusterLicense
	err = reconciler.ReconcileResource(reconciler.Params{
		Client:     r,
		Scheme:     r.scheme,
		Owner:      &cluster,
		Expected:   toAssign,
		Reconciled: &reconciled,
		NeedsUpdate: func() bool {
			return !reconciled.IsValid(time.Now(), margin)
		},
		UpdateReconciled: func() {

			reconciled.Spec = toAssign.Spec
		},
		OnCreate: func() {
			log.Info("Assigning license", "cluster", clusterName, "license", match.Spec.UID, "expiry", match.ExpiryDate())
		},
		OnUpdate: func() {
			log.Info("Updating license to", "cluster", clusterName, "license", match.Spec.UID, "expiry", match.ExpiryDate())
		},
	})
	return match.ExpiryDate(), err
}

// Reconcile reads the cluster license for the cluster being reconciled. If found, it checks whether it is still valid.
// If there is none it assigns a new one.
// In any case it schedules a new reconcile request to be processed when the license is about to expire.
// This happens independently from any watch triggered reconcile request.
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=enterpriselicenses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=elasticsearchclusters,verbs=get;list;watch
func (r *ReconcileLicenses) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cluster to ensure it still exists
	owner := v1alpha1.ElasticsearchCluster{}
	err := r.Get(newContext(), request.NamespacedName, &owner)
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
	safetyMargin := defaultSafetyMargin()
	log.Info("Reconciling licenses", "cluster", request.NamespacedName)
	newExpiry, err := r.reconcileClusterLicense(owner, safetyMargin)
	if err != nil {
		log.Info("Re-queuing new license check immediately (rate-limited)", "cluster", request.NamespacedName)
		return reconcile.Result{Requeue: true}, err
	}
	result := nextReconcile(newExpiry, safetyMargin)
	log.Info("Re-queuing new license check", "cluster", request.NamespacedName, "RequeueAfter", result.RequeueAfter)
	return result, nil
}
