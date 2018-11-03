package stack

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
	// watch any pods created by Stack - change this for objects you create
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
	res, err = r.reconcileService(&stack, elasticsearch.NewDiscoveryService(stack))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(&stack, elasticsearch.NewPublicService(stack))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileKibanaDeployment(&stack)
	if err != nil {
		return res, err
	}

	return r.DeleteElasticsearchPods(request)
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(request reconcile.Request) (deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(context.TODO(), request.NamespacedName, &stackInstance); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return stackInstance, nil
		}
		// Error reading the object - requeue the request.
		return stackInstance, err
	}
	return stackInstance, nil
}

// GetPodList returns PodList in the current namespace with a specific set of
// filters (labels and fields).
func (r *ReconcileStack) GetPodList(request reconcile.Request, labelSelectors, fieldSelectors map[string]string) (corev1.PodList, error) {
	var podList corev1.PodList
	stack, err := r.GetStack(request)
	if err != nil {
		return podList, err
	}

	var rawLabelSelectors strings.Builder
	rawLabelSelectors.WriteString(elasticsearch.ClusterIDLabelName)
	rawLabelSelectors.WriteString("=")
	rawLabelSelectors.WriteString(common.StackID(stack))
	for k, v := range labelSelectors {
		rawLabelSelectors.WriteString(",")
		rawLabelSelectors.WriteString(k)
		rawLabelSelectors.WriteString("=")
		rawLabelSelectors.WriteString(v)
	}

	var rawFieldSelectors strings.Builder
	for k, v := range fieldSelectors {
		if rawFieldSelectors.Len() > 0 {
			rawFieldSelectors.WriteString(",")
		}
		rawLabelSelectors.WriteString(k)
		rawLabelSelectors.WriteString("=")
		rawLabelSelectors.WriteString(v)
	}

	var listOpts = client.ListOptions{Namespace: request.Namespace}
	if err := listOpts.SetLabelSelector(rawLabelSelectors.String()); err != nil {
		return podList, err
	}

	if rawFieldSelectors.Len() > 0 {
		if err := listOpts.SetFieldSelector(rawFieldSelectors.String()); err != nil {
			return podList, err
		}
	}

	if err := r.List(context.TODO(), &listOpts, &podList); err != nil {
		return podList, err
	}

	return podList, nil
}

// CreateElasticsearchPods Performs the creation of any number of pods in order
// to match the Stack definition.
func (r *ReconcileStack) CreateElasticsearchPods(request reconcile.Request) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	currentPods, err := r.GetPodList(request, elasticsearch.TypeFilter, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	var podSpecParams = elasticsearch.BuildNewPodSpecParams(stackInstance)
	var proposedPods []corev1.Pod

	// Create any missing instances
	for i := int32(len(currentPods.Items)); i < stackInstance.Spec.Elasticsearch.NodeCount; i++ {
		pod := elasticsearch.NewPod(stackInstance)

		if err := controllerutil.SetControllerReference(&stackInstance, &pod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		proposedPods = append(proposedPods, pod)
	}

	// Any pods with different spec hashes need to be recreated.
	for _, pod := range currentPods.Items {
		h, ok := pod.Labels[elasticsearch.HashLabelName]
		if !ok {
			return reconcile.Result{}, nil
		}

		// On equal hashes return, all is good!
		if h == podSpecParams.Hash() || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		if tainted, ok := pod.Labels[elasticsearch.TaintedLabelName]; ok {
			if t, _ := strconv.ParseBool(tainted); t {
				continue
			}
		}

		// Mark the pod as tainted.
		pod.Labels[elasticsearch.TaintedLabelName] = "true"
		if err := r.Update(context.TODO(), &pod); err != nil {
			return reconcile.Result{}, err
		}

		newPod := elasticsearch.NewPod(stackInstance)
		if err := controllerutil.SetControllerReference(&stackInstance, &newPod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		proposedPods = append(proposedPods, newPod)
	}

	// Trim the # of proposed pods to the node count, we can't have more nodes
	// being created > NodeCount. This is required to not do work in vain when
	// there's a decrease in the number of nodes in the topology and a hash
	// change.
	if int32(len(proposedPods)) > stackInstance.Spec.Elasticsearch.NodeCount {
		proposedPods = proposedPods[:stackInstance.Spec.Elasticsearch.NodeCount]
	}

	for _, pod := range proposedPods {
		if err := r.Create(context.TODO(), &pod); err != nil {
			return reconcile.Result{}, err
		}
		log.Info(common.Concat("Created Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
	}

	return reconcile.Result{}, err
}

// DeleteElasticsearchPods removes running pods to match the Stack definition.
func (r *ReconcileStack) DeleteElasticsearchPods(request reconcile.Request) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Get the current list of instances
	currentPods, err := r.GetPodList(request, elasticsearch.TypeFilter, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Sort pods by age.
	sort.SliceStable(currentPods.Items, func(i, j int) bool {
		return currentPods.Items[i].CreationTimestamp.Before(&currentPods.Items[j].CreationTimestamp)
	})

	// Delete the difference between the running and desired pods.
	var orphanPodNumber = int32(len(currentPods.Items)) - stackInstance.Spec.Elasticsearch.NodeCount
	for i := int32(0); i < orphanPodNumber; i++ {
		var pod = currentPods.Items[i]
		if pod.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
		if pod.Status.Phase == corev1.PodRunning {
			// TODO: Handle migration here before we delete the pod.
			for _, c := range pod.Status.Conditions {
				// Return when the pod is not Ready (API Unreachable).
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionFalse {
					return reconcile.Result{}, nil
				}
			}

			if err := r.Delete(context.TODO(), &pod); err != nil && !errors.IsNotFound(err) {
				return reconcile.Result{}, err
			}
			log.Info(common.Concat("Deleted Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		}

	}
	return reconcile.Result{}, nil
}

func (r *ReconcileStack) reconcileKibanaDeployment(stack *deploymentsv1alpha1.Stack) (reconcile.Result, error) {
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		CustomImageName:  stack.Spec.Kibana.Image,
		ElasticsearchUrl: elasticsearch.PublicServiceURL(stack.Name),
	}
	labels := kibana.NewLabelsWithStackID(common.StackID(*stack))
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
