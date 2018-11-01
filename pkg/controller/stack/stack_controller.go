package stack

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// TODO(user): Modify this to be the types you create
	// Uncomment watch pods created by Stack - change this for objects you create
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
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
	// To support concurrent runs.
	atomic.AddInt64(&r.iteration, 1)
	res, err := r.CreateElasticsearchPods(request)
	if err != nil {
		return res, err
	}

	stack, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	clusterID := elasticsearch.ClusterID(stack.Namespace, stack.Name)
	res, err = r.reconcileEsDeployment(stack, clusterID)
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(stack, elasticsearch.NewDiscoveryService(stack.Namespace, stack.Name, stackID))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(stack, elasticsearch.NewPublicService(stack.Namespace, stack.Name, stackID))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileKibanaDeployment(stack, stackID)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileService(stack, kibana.NewService(stack.Namespace, stack.Name, stackID))
	if err != nil {
		return res, err
	}

	return r.DeleteElasticsearchPods(request)
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(request reconcile.Request) (*deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(context.TODO(), request.NamespacedName, &stackInstance); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil, nil
		}
		// Error reading the object - requeue the request.
		return nil, err
	}
	return &stackInstance, nil
}

// CreateElasticsearchPods performs the creation of any number of pods in order
// to match the Stack definition.
func (r *ReconcileStack) CreateElasticsearchPods(request reconcile.Request) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create any missing instances
	var current = stackInstance.Status.Elasticsearch.Nodes
	var desired = stackInstance.Spec.Elasticsearch.NodeCount
	for index := current; index < desired; index++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      elasticsearch.NextNodeName(*stackInstance),
				Namespace: stackInstance.Namespace,
			},
			Spec: elasticsearch.NewPodSpec(elasticsearch.NewPodSpecParams{
				Version:                        stackInstance.Spec.Version,
				CustomImageName:                stackInstance.Spec.Elasticsearch.Image,
				ClusterName:                    stackInstance.Name,
				DiscoveryZenMinimumMasterNodes: 1,
				DiscoveryServiceName:           "localhost",
				SetVmMaxMapCount:               stackInstance.Spec.Elasticsearch.SetVmMaxMapCount,
			}),
		}

		if err := controllerutil.SetControllerReference(stackInstance, &pod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		// Check if the pod already exists
		var found corev1.Pod
		var namespaceFilter = types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}
		// If the pod exists, continue with the loop.
		if err = r.Get(context.TODO(), namespaceFilter, &found); err == nil {
			continue
		}

		log.Info(
			fmt.Sprint("Creating Pod ", pod.Namespace, pod.Name),
			"iteration", atomic.LoadInt64(&r.iteration),
		)
		err = r.Create(context.TODO(), &pod)
		if err != nil {
			return reconcile.Result{}, err
		}

		stackInstance.Status.Elasticsearch.NodeAdded()
		if err := r.Update(context.TODO(), stackInstance); err != nil {
			return reconcile.Result{}, err
		}

		// We don't update pods in place.
		// TODO: Decide what to do when container settings are updated.
		// Find a good comparable way to compare instances.
	}

	return reconcile.Result{}, err
}

// DeleteElasticsearchPods removes running pods to match the Stack definition.
func (r *ReconcileStack) DeleteElasticsearchPods(request reconcile.Request) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	var desired = stackInstance.Spec.Elasticsearch.NodeCount
	var current = stackInstance.Status.Elasticsearch.Nodes
	for index := desired; index < current; index++ {
		var pod corev1.Pod
		var key = types.NamespacedName{
			Name:      elasticsearch.FirstNodName(*stackInstance),
			Namespace: stackInstance.Namespace,
		}

		if err := r.Get(context.TODO(), key, &pod); err != nil {
			log.Info("No pods to delete", "iteration", atomic.LoadInt64(&r.iteration))
			if err := r.Update(context.TODO(), stackInstance); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		// TODO: Handle migration here.
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady {
				if c.Status == corev1.ConditionFalse {
					log.Info(
						fmt.Sprint("Pod ", pod.Name, " is not ready, requeuing..."),
						"iteration", atomic.LoadInt64(&r.iteration),
					)
					// TODO: Create circuit breaker (M).
					return reconcile.Result{Requeue: true, RequeueAfter: time.Second}, nil
				}
			}
		}

		log.Info(fmt.Sprint("Deleting Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		if err := r.Delete(context.TODO(), &pod); err != nil {
			return reconcile.Result{}, err
		}

		log.Info(fmt.Sprint("Deleted Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		stackInstance.Status.Elasticsearch.NodeDeleted()
		if err := r.Update(context.TODO(), stackInstance); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileStack) reconcileEsDeployment(stack *deploymentsv1alpha1.Stack, stackID string) (reconcile.Result, error) {
	esPodSpecParams := elasticsearch.NewPodSpecParams{
		Version:                        stack.Spec.Version,
		CustomImageName:                stack.Spec.Elasticsearch.Image,
		ClusterName:                    stackID,
		DiscoveryZenMinimumMasterNodes: elasticsearch.ComputeMinimumMasterNodes(int(stack.Spec.Elasticsearch.NodeCount)),
		DiscoveryServiceName:           elasticsearch.DiscoveryServiceName(stack.Name),
		SetVmMaxMapCount:               stack.Spec.Elasticsearch.SetVmMaxMapCount,
	}

	labels := elasticsearch.NewLabelsWithStackID(stackID)
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

func (r *ReconcileStack) reconcileKibanaDeployment(stack *deploymentsv1alpha1.Stack, stackID string) (reconcile.Result, error) {
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		CustomImageName:  stack.Spec.Kibana.Image,
		ElasticsearchUrl: elasticsearch.PublicServiceURL(stack.Name),
	}
	labels := kibana.NewLabelsWithStackID(stackID)
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
