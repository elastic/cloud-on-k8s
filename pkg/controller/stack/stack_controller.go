package stack

import (
	"context"
	"fmt"
	"reflect"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	// res, err = r.reconcileKibanaDeployment(stack, clusterID)
	// if err != nil {
	// 	return res, err
	// }

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

	// Define the desired Deployment object
	// NewDeployment(name string, namespace string, selector map[string]string, labels map[string]string, replicas int, spec corev1.PodSpec)
	labels := elasticsearch.ClusterIDLabels(clusterID)
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

func (r *ReconcileStack) reconcileKibanaDeployment(esDeployment appsv1.Deployment, stack *deploymentsv1alpha1.Stack, clusterID string) (reconcile.Result, error) {

	//TODO use a service
	pods := &corev1.PodList{}
	err := r.List(context.TODO(), client.MatchingLabels(esDeployment.Spec.Template.ObjectMeta.Labels), pods)
	if err != nil || len(pods.Items) == 0 {
		log.Info(fmt.Sprintf("Pods %s not found. Re-queueing", esDeployment.Spec.Template.ObjectMeta.Labels))
		return reconcile.Result{Requeue: true}, err
	}

	host := pods.Items[0].Status.PodIP

	elasticsearchURL := fmt.Sprintf("http://%s:9200", host)

	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		ElasticsearchUrl: elasticsearchURL,
	}
	deploy := NewDeployment(DeploymentParams{
		Name:      kibana.NewDeploymentName(stack.Name),
		Namespace: stack.Namespace,
		Replicas:  1,
		PodSpec:   kibana.NewPodSpec(kibanaPodSpecParams),
	})
	return r.ReconcileDeployment(deploy, *stack)
}

func (r *ReconcileStack) reconcileService(stack *deploymentsv1alpha1.Stack, service *corev1.Service) (reconcile.Result, error) {
	if err := controllerutil.SetControllerReference(stack, service, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Check if already exists
	expected := service
	found := &corev1.Service{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: expected.Name, Namespace: expected.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Create if needed
		log.Info(fmt.Sprintf("Creating service %s/%s\n", expected.Namespace, expected.Name),
			"iteration", r.iteration,
		)

		err = r.Create(context.TODO(), expected)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	// ClusterIP might not exist in the expected service,
	// but might have been set after creation by k8s on the actual resource.
	// In such case, we want to use these values for comparison.
	if expected.Spec.ClusterIP == "" {
		expected.Spec.ClusterIP = found.Spec.ClusterIP
	}
	// same for the target port and node port
	if len(expected.Spec.Ports) == len(found.Spec.Ports) {
		for i := range expected.Spec.Ports {
			if expected.Spec.Ports[i].TargetPort.IntValue() == 0 {
				expected.Spec.Ports[i].TargetPort = found.Spec.Ports[i].TargetPort
			}
			if expected.Spec.Ports[i].NodePort == 0 {
				expected.Spec.Ports[i].NodePort = found.Spec.Ports[i].NodePort
			}
		}
	}

	// Update if needed
	if !reflect.DeepEqual(expected.Spec, found.Spec) {
		log.Info(
			fmt.Sprintf("Updating service %s/%s\n", expected.Namespace, expected.Name),
			"iteration", r.iteration,
		)
		found.Spec = expected.Spec // only update spec, keep the rest
		err := r.Update(context.TODO(), found)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}
