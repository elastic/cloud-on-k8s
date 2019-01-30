package license

import (
	"context"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/operator"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("license-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to EnterpriseLicense
	err = c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.EnterpriseLicense{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
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

// Reconcile reads that state of the cluster for a license object and makes changes based on the state read
// and what is in the license spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=enterpriselicenses,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileLicenses) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the EnterpriseLicense instance
	instance := &elasticsearchv1alpha1.EnterpriseLicense{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// TODO implement reconcile
	return reconcile.Result{}, nil
}
