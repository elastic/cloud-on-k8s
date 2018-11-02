package stack

import (
	"context"

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

	// TODO: filter those types to make sure we don't watch *all* deployments and services in the cluster
	// Watch deployments
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &deploymentsv1alpha1.Stack{},
	})
	if err != nil {
		return err
	}
	// Watch services
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &deploymentsv1alpha1.Stack{},
	})

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
	stack := &deploymentsv1alpha1.Stack{}
	err := r.Get(context.TODO(), request.NamespacedName, stack)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	clusterID := elasticsearch.ClusterID(stack.Namespace, stack.Name)

	res, err := r.reconcileEsDeployment(stack, clusterID)
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(stack, elasticsearch.NewDiscoveryService(stack.Namespace, stack.Name, clusterID))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(stack, elasticsearch.NewPublicService(stack.Namespace, stack.Name, clusterID))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileKibanaDeployment(stack, clusterID)
	if err != nil {
		return res, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileStack) reconcileEsDeployment(stack *deploymentsv1alpha1.Stack, clusterID string) (reconcile.Result, error) {
	esPodSpecParams := elasticsearch.NewPodSpecParams{
		Version:                        stack.Spec.Version,
		CustomImageName:                stack.Spec.Elasticsearch.Image,
		ClusterName:                    clusterID,
		DiscoveryZenMinimumMasterNodes: elasticsearch.ComputeMinimumMasterNodes(int(stack.Spec.Elasticsearch.NodeCount)),
		DiscoveryServiceName:           elasticsearch.DiscoveryServiceName(stack.Name),
		SetVmMaxMapCount:               stack.Spec.Elasticsearch.SetVmMaxMapCount,
	}

	labels := elasticsearch.NewLabelsWithClusterID(clusterID)
	deploy := NewDeployment(DeploymentParams{
		Name:      stack.Name + "-es",
		Namespace: stack.Namespace,
		Selector:  labels,
		Labels:    labels,
		Replicas:  stack.Spec.Elasticsearch.NodeCount,
		PodSpec:   elasticsearch.NewPodSpec(esPodSpecParams),
	})
	res, err := r.ReconcileDeployment(deploy, *stack)
	if err != nil {
		return res, err
	}
	return res, nil
}

func (r *ReconcileStack) reconcileKibanaDeployment(stack *deploymentsv1alpha1.Stack, clusterID string) (reconcile.Result, error) {
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		ElasticsearchUrl: elasticsearch.PublicServiceURL(stack.Name),
	}
	labels := kibana.NewLabelsWithClusterID(clusterID)
	deploy := NewDeployment(DeploymentParams{
		Name:      kibana.NewDeploymentName(stack.Name),
		Namespace: stack.Namespace,
		Replicas:  1,
		Selector:  labels,
		Labels:    labels,
		PodSpec:   kibana.NewPodSpec(kibanaPodSpecParams),
	})
	return r.ReconcileDeployment(deploy, *stack)
}
