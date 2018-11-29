package stack

import (
	"context"
	"sync/atomic"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	log            = logf.Log.WithName("stack-controller")
)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this deployments.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	r, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, r)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	esCa, err := nodecerts.NewSelfSignedCa("stack-controller elasticsearch")
	if err != nil {
		return nil, err
	}

	kibanaCa, err := nodecerts.NewSelfSignedCa("stack-controller kibana")
	if err != nil {
		return nil, err
	}

	return &ReconcileStack{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		esCa:     esCa,
		kibanaCa: kibanaCa,
		recorder: mgr.GetRecorder("stack-controller"),
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("stack-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Elasticsearch
	err = c.Watch(&source.Kind{Type: &deploymentsv1alpha1.Stack{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileStack{}

// ReconcileStack reconciles a Elasticsearch object
type ReconcileStack struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	esCa     *nodecerts.Ca
	kibanaCa *nodecerts.Ca

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and what is in
// the Elasticsearch.Spec
//
// Automatically generate RBAC rules:
// +kubebuilder:rbac:groups=deployments.k8s.elastic.co,resources=stacks;stacks/status,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileStack) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	stack, err := r.GetStack(request.NamespacedName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// TODO: actually do something..
	_ = stack

	return reconcile.Result{}, nil
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(name types.NamespacedName) (deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(context.TODO(), name, &stackInstance); err != nil {
		return stackInstance, err
	}
	return stackInstance, nil
}
