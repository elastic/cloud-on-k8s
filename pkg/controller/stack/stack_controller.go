package stack

import (
	"context"
	"fmt"
	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

var log = logf.Log.WithName("stack-controller")

// Add creates a new Stack Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this deployments.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileStack{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("stack-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Stack
	err = c.Watch(&source.Kind{Type: &deploymentsv1alpha1.Stack{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by Stack - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &deploymentsv1alpha1.Stack{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileStack{}

// ReconcileStack reconciles a Stack object
type ReconcileStack struct {
	client.Client
	scheme *runtime.Scheme

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Stack object and makes changes based on the state read
// and what is in the Stack.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployments.k8s.elastic.co,resources=stacks,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileStack) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	r.iteration += 1

	// Fetch the Stack instance
	instance := &deploymentsv1alpha1.Stack{}
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

	// TODO(user): Change this to be the object type created by your controller
	esPodSpecParams := elasticsearch.NewPodSpecParams{
		Version:                        instance.Spec.Version,
		CustomImageName:                instance.Spec.Elasticsearch.Image,
		ClusterName:                    instance.Name,
		DiscoveryZenMinimumMasterNodes: 1,
		DiscoveryServiceName:           "localhost",
		SetVmMaxMapCount:               instance.Spec.Elasticsearch.SetVmMaxMapCount,
	}

	// Define the desired Deployment object
	deploy := NewDeployment(instance.Name, instance.Namespace, elasticsearch.NewPodSpec(esPodSpecParams))
	if result, err := r.ReconcileDeployment(deploy, *instance); err != nil {
		return result, err
	}

	//TODO use a service
	pods := &corev1.PodList{}
	err = r.List(context.TODO(), client.MatchingLabels(deploy.Spec.Template.ObjectMeta.Labels), pods)
	if err != nil || len(pods.Items) == 0 {
		log.Info(fmt.Sprintf("Pods %s not found. Re-queueing", deploy.Spec.Template.ObjectMeta.Labels))
		return reconcile.Result{Requeue: true}, err
	}

	host := pods.Items[0].Status.PodIP

	elasticsearchUrl := fmt.Sprintf("http://%s:9200", host)
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          instance.Spec.Version,
		ElasticsearchUrl: elasticsearchUrl,
	}
	deploy = NewDeployment(
		kibana.NewDeploymentName(instance.Name),
		instance.Namespace,
		kibana.NewPodSpec(kibanaPodSpecParams))
	return r.ReconcileDeployment(deploy, *instance)

}
