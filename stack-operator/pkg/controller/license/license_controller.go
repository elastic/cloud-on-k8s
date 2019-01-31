package license

import (
	"context"
	"fmt"
	"time"

	v1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	match "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/license"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/errors"
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
		kind = match.DesiredLicenseType(&s)
	}

	licenseList := v1alpha1.EnterpriseLicenseList{}
	err = c.List(newContext(), &client.ListOptions{}, &licenseList)
	if err != nil {
		return noLicense, err
	}
	bestMatch := match.BestMatch(licenseList.Items, kind)
	if bestMatch == nil {
		return noLicense, fmt.Errorf("could not find a matching license for %v", clusterName)
	}
}

func assignLicense(c client.Client, clusterName types.NamespacedName) error {
	match, err := findLicenseFor(c, clusterName)
	if err != nil {
		return err
	}
	toAssign := match.DeepCopy()
	toAssign.ObjectMeta = k8s.ToObjectMeta(clusterName)
	return c.Create(newContext(), toAssign)
}

func reassignLicense(c client.Client, clusterName types.NamespacedName) error {
	match, err := findLicenseFor(c, clusterName)
	if err != nil {
		return err
	}
	existing := v1alpha1.ClusterLicense{}
	err = c.Get(newContext(), clusterName, &existing)
	if err != nil {
		return err
	}
	existing.Spec = match.Spec
	return c.Update(newContext(), &existing)
}

// Reconcile reads that state of the cluster for a license object and makes changes based on the state read
// and what is in the license spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=enterpriselicenses,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileLicenses) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cluster license in the namespace of the cluster
	instance := &v1alpha1.ClusterLicense{}
	err := r.Get(newContext(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			err := assignLicense(r, request.NamespacedName)
			if err != nil {
				return reconcile.Result{Requeue: true}, err
			}
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	if !instance.IsValidAt(time.Now(), v1alpha1.NewSafetyMargin()) {
		err := reassignLicense(r, request.NamespacedName)
		if err != nil {
			return reconcile.Result{Requeue: true}, err
		}
		return reconcile.Result{}, nil
	}

	// nothing to do
	return reconcile.Result{}, nil
}
