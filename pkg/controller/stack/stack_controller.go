package stack

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	esclient "github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/util/rand"
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

	stack, err := r.GetStack(request)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	res, err := r.CreateElasticsearchPods(request)
	if err != nil {
		return res, err
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

	res, err = r.reconcileService(&stack, kibana.NewService(stack))
	if err != nil {
		return res, err
	}

	return r.DeleteElasticsearchPods(request)
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(request reconcile.Request) (deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(context.TODO(), request.NamespacedName, &stackInstance); err != nil {
		return stackInstance, err
	}
	return stackInstance, nil
}

// NewElasticsearchClient creates a new client bound to the given stack instance.
func NewElasticsearchClient(stack *deploymentsv1alpha1.Stack) (*esclient.Client, error) {
	esURL, err := elasticsearch.ExternalServiceURL(stack.Name)
	return &esclient.Client{
		Endpoint: esURL,
		HTTP:     &http.Client{},
	}, err
}

// GetPodList returns PodList in the current namespace with a specific set of
// filters (labels and fields).
func (r *ReconcileStack) GetPodList(request reconcile.Request, labelSelectors labels.Selector, fieldSelectors fields.Selector) (corev1.PodList, error) {
	var podList corev1.PodList
	stack, err := r.GetStack(request)
	if err != nil {
		return podList, err
	}

	// add a label for the cluster ID
	clusterIDReq, err := labels.NewRequirement(elasticsearch.ClusterIDLabelName, selection.Equals, []string{common.StackID(stack)})
	if err != nil {
		return podList, err
	}
	labelSelectors.Add(*clusterIDReq)

	listOpts := client.ListOptions{
		Namespace:     request.Namespace,
		LabelSelector: labelSelectors,
		FieldSelector: fieldSelectors,
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

	currentPods, err := r.GetPodList(request, elasticsearch.TypeSelector, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	var podSpecParams = elasticsearch.BuildNewPodSpecParams(stackInstance)
	var proposedPods []corev1.Pod

	// Create any missing instances
	for i := int32(len(currentPods.Items)); i < stackInstance.Spec.Elasticsearch.NodeCount(); i++ {
		pod, err := elasticsearch.NewPod(stackInstance)
		if err != nil {
			return reconcile.Result{}, err
		}

		if err := controllerutil.SetControllerReference(&stackInstance, &pod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		proposedPods = append(proposedPods, pod)
	}

	// Any pods with different spec hashes need to be recreated.
	for _, pod := range currentPods.Items {
		h, ok := pod.Labels[elasticsearch.HashLabelName]
		if !ok {
			continue
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

		newPod, err := elasticsearch.NewPod(stackInstance)
		if err != nil {
			return reconcile.Result{}, err
		}
		if err := controllerutil.SetControllerReference(&stackInstance, &newPod, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		proposedPods = append(proposedPods, newPod)
	}

	// Trim the # of proposed pods to the node count, we can't have more nodes
	// being created > NodeCount. This is required to not do work in vain when
	// there's a decrease in the number of nodes in the topology and a hash
	// change.
	if int32(len(proposedPods)) > stackInstance.Spec.Elasticsearch.NodeCount() {
		proposedPods = proposedPods[:stackInstance.Spec.Elasticsearch.NodeCount()]
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
	currentPods, err := r.GetPodList(request, elasticsearch.TypeSelector, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Sort pods by age.
	sort.SliceStable(currentPods.Items, func(i, j int) bool {
		return currentPods.Items[i].CreationTimestamp.Before(&currentPods.Items[j].CreationTimestamp)
	})

	// Delete the difference between the running and desired pods.
	var orphanPodNumber = int32(len(currentPods.Items)) - stackInstance.Spec.Elasticsearch.NodeCount()
	var toDelete []corev1.Pod
	var nodeNames []string
	for i := int32(0); i < orphanPodNumber; i++ {
		var pod = currentPods.Items[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning {
			for _, c := range pod.Status.Conditions {
				// Return when the pod is not Ready (API Unreachable).
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionFalse {
					continue
				}
			}
			toDelete = append(toDelete, pod)
			nodeNames = append(nodeNames, pod.Name)
		}

	}

	//create an Elasticsearch client
	esClient, err := NewElasticsearchClient(&stackInstance)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "Could not create ES client")
	}

	if err = elasticsearch.MigrateData(esClient, nodeNames); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "Error during migrate data")
	}

	for _, pod := range toDelete {
		isMigratingData, err := elasticsearch.IsMigratingData(esClient, pod)
		if err != nil {
			return reconcile.Result{}, err
		}
		if isMigratingData {
			log.Info(common.Concat("Migrating data, skipping deletes because of ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
			r := time.Duration(rand.Intn(60)) * time.Second // TODO make this dependent on the amount of data to migrate
			return reconcile.Result{Requeue: true, RequeueAfter: r}, nil
		}

		if err := r.Delete(context.TODO(), &pod); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		log.Info(common.Concat("Deleted Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))

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
