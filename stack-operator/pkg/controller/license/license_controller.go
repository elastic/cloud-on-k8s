package license

import (
	"context"
	"reflect"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	match "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
	return reconcile.Result{
		// requeue at expiry minus safetyMargin/2 to ensure we actually reissue a license on the next attempt
		RequeueAfter: time.Until(expiry.Add(-1 * (safety.ValidFor / 2))),
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

// ReconcileLicenses reconciles a EnterpriseLicense object
type ReconcileLicenses struct {
	client.Client
	scheme *runtime.Scheme
}

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

func assignLicense(c client.Client, clusterName types.NamespacedName) (time.Time, error) {
	var noResult time.Time
	match, err := findLicenseFor(c, clusterName)
	if err != nil {
		return noResult, err
	}
	toAssign := match.DeepCopy()
	toAssign.ObjectMeta = k8s.ToObjectMeta(clusterName)
	err = setOwnerReference(c, toAssign, clusterName)
	if err != nil {
		return noResult, err
	}
	return match.ExpiryDate(), c.Create(newContext(), toAssign)
}

func setOwnerReference(c client.Client, clusterLicense *v1alpha1.ClusterLicense, clusterName types.NamespacedName) error {
	owner := v1alpha1.ElasticsearchCluster{}
	err := c.Get(newContext(), clusterName, &owner)
	if err != nil {
		return err
	}
	gvk := owner.GetObjectKind().GroupVersionKind()
	blockOwnerDeletion := false
	isController := false
	ownerRef := v1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}

	if owner.DeletionTimestamp.IsZero() {

		existing := clusterLicense.GetOwnerReferences()
		for _, r := range existing {
			if reflect.DeepEqual(r, ownerRef) {
				return nil
			}
		}
		existing = append(existing, ownerRef)
		clusterLicense.SetOwnerReferences(existing)
		return nil
	}
	return nil
}

func reassignLicense(c client.Client, clusterName types.NamespacedName) (time.Time, error) {
	var noResult time.Time
	match, err := findLicenseFor(c, clusterName)
	if err != nil {
		return noResult, err
	}
	existing := v1alpha1.ClusterLicense{}
	err = c.Get(newContext(), clusterName, &existing)
	if err != nil {
		return noResult, err
	}
	existing.Spec = match.Spec
	return match.ExpiryDate(), c.Update(newContext(), &existing)
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
	// Fetch the cluster license in the namespace of the cluster
	safetyMargin := defaultSafetyMargin()
	instance := &v1alpha1.ClusterLicense{}
	err := r.Get(newContext(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			newExpiry, err := assignLicense(r, request.NamespacedName)
			if err != nil {
				return reconcile.Result{Requeue: true}, err
			}
			return nextReconcile(newExpiry, safetyMargin), nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if !instance.IsValidAt(time.Now(), safetyMargin) {
		newExpiry, err := reassignLicense(r, request.NamespacedName)
		if err != nil {
			return reconcile.Result{Requeue: true}, err
		}
		return nextReconcile(newExpiry, safetyMargin), nil
	}

	// nothing but reschedule/update any previously scheduled items
	return nextReconcile(instance.ExpiryDate(), safetyMargin), nil
}
