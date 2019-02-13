// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stack

import (
	"fmt"
	"reflect"
	"sync/atomic"
	"time"

	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	deploymentsv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	v1alpha12 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/k8s-operators/operators/pkg/utils/diff"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log            = logf.Log.WithName("stack-controller")
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this deployments.Add(mgr) to install this Controller
func Add(mgr manager.Manager, _ operator.Parameters) error {
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
		Client:   k8s.WrapClient(mgr.GetClient()),
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

	// Watch for changes to the Stack
	if err := c.Watch(&source.Kind{Type: &deploymentsv1alpha1.Stack{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch elasticsearch cluster objects
	if err := c.Watch(&source.Kind{Type: &v1alpha1.ElasticsearchCluster{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &deploymentsv1alpha1.Stack{},
	}); err != nil {
		return err
	}

	// Watch kibana objects
	if err := c.Watch(&source.Kind{Type: &v1alpha12.Kibana{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &deploymentsv1alpha1.Stack{},
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileStack{}

// ReconcileStack reconciles a Elasticsearch object
type ReconcileStack struct {
	k8s.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	esCa     *nodecerts.Ca
	kibanaCa *nodecerts.Ca

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and what is in
// the Elasticsearch.Spec
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

	if common.IsPaused(stack.ObjectMeta, r.Client) {
		log.Info("Paused : skipping reconciliation", "iteration", currentIteration)
		return common.PauseRequeue, nil
	}

	// use the same name for es and kibana resources for now
	esAndKbKey := types.NamespacedName{Namespace: stack.Namespace, Name: stack.Name}

	es := v1alpha1.ElasticsearchCluster{
		ObjectMeta: k8s.ToObjectMeta(esAndKbKey),
		Spec:       stack.Spec.Elasticsearch,
	}

	if es.Spec.Version == "" {
		es.Spec.Version = stack.Spec.Version
	}

	// TODO this merging of feature flags look ripe for a generalized function
	if stack.Spec.FeatureFlags != nil {
		if es.Spec.FeatureFlags == nil {
			es.Spec.FeatureFlags = make(commonv1alpha1.FeatureFlags, len(stack.Spec.FeatureFlags))
		}
		for k, v := range stack.Spec.FeatureFlags {
			if _, ok := es.Spec.FeatureFlags[k]; !ok {
				es.Spec.FeatureFlags[k] = v
			}
		}
	}

	// initially sync labels from stack resource to Elasticsearch (mainly to propagate licensing labels atm)
	if len(stack.Labels) > 0 {
		if es.Labels == nil {
			es.Labels = make(map[string]string, len(stack.Labels))
		}
		for k, v := range stack.Labels {
			if _, ok := es.Labels[k]; !ok {
				es.Labels[k] = v
			}
		}
	}

	if err := controllerutil.SetControllerReference(&stack, &es, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	var currentEs v1alpha1.ElasticsearchCluster
	if err := r.Get(esAndKbKey, &currentEs); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	if currentEs.UID == "" {
		log.Info("Creating ElasticsearchCluster spec")
		if err := r.Create(&es); err != nil {
			return reconcile.Result{}, err
		}
		currentEs = es
	} else {
		// TODO: this is a bit rough
		if err := diff.NewDiffAsError(currentEs.Spec, es.Spec); err != nil {
			log.Info("Updating ElasticsearchCluster spec")

			log.V(4).Info(
				"Updating ElasticsearchCluster spec due to changes",
				"diff_err", err,
			)

			currentEs.Spec = es.Spec
			if err := r.Update(&currentEs); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	kb := v1alpha12.Kibana{
		ObjectMeta: k8s.ToObjectMeta(esAndKbKey),
		Spec:       stack.Spec.Kibana,
	}

	if kb.Spec.Version == "" {
		kb.Spec.Version = stack.Spec.Version
	}

	// TODO this merging of feature flags look ripe for a generalized function
	if stack.Spec.FeatureFlags != nil {
		if kb.Spec.FeatureFlags == nil {
			kb.Spec.FeatureFlags = make(commonv1alpha1.FeatureFlags, len(stack.Spec.FeatureFlags))
		}
		for k, v := range stack.Spec.FeatureFlags {
			if _, ok := kb.Spec.FeatureFlags[k]; !ok {
				kb.Spec.FeatureFlags[k] = v
			}
		}
	}

	if err := controllerutil.SetControllerReference(&stack, &kb, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// TODO: be dynamic wrt to the service name
	kb.Spec.Elasticsearch.URL = fmt.Sprintf("https://%s:9200", services.ExternalServiceName(es.Name))

	internalUsersSecretName := secret.ElasticInternalUsersSecretName(es.Name)
	var internalUsersSecret corev1.Secret
	internalUsersSecretKey := types.NamespacedName{Namespace: stack.Namespace, Name: internalUsersSecretName}
	if err := r.Get(internalUsersSecretKey, &internalUsersSecret); err != nil {
		return reconcile.Result{}, err
	}

	// TODO: can deliver through a shared secret instead?
	kb.Spec.Elasticsearch.Auth.Inline = &v1alpha12.ElasticsearchInlineAuth{
		Username: secret.InternalKibanaServerUserName,
		// TODO: error checking
		Password: string(internalUsersSecret.Data[secret.InternalKibanaServerUserName]),
	}

	var publicCACertSecret corev1.Secret
	publicCACertSecretKey := types.NamespacedName{Namespace: stack.Namespace, Name: es.Name}
	if err = r.Get(publicCACertSecretKey, &publicCACertSecret); err != nil {
		return defaultRequeue, err // maybe not created yet
	}
	kb.Spec.Elasticsearch.CaCertSecret = &publicCACertSecret.Name

	var currentKb v1alpha12.Kibana
	if err := r.Get(esAndKbKey, &currentKb); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	if currentKb.UID == "" {
		log.Info("Creating Kibana spec")
		if err := r.Create(&kb); err != nil {
			return reconcile.Result{}, err
		}

		currentKb = kb
	} else {
		// TODO: this is a bit rough
		if !reflect.DeepEqual(currentKb.Spec, kb.Spec) {
			currentKb.Spec = kb.Spec
			log.Info("Updating Kibana spec")
			if err := r.Update(&currentKb); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// maybe update status
	origStatus := stack.Status.DeepCopy()
	stack.Status.Elasticsearch = currentEs.Status
	stack.Status.Kibana = currentKb.Status

	if !reflect.DeepEqual(*origStatus, stack.Status) {
		if err := r.Status().Update(&stack); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(name types.NamespacedName) (deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(name, &stackInstance); err != nil {
		return stackInstance, err
	}
	return stackInstance, nil
}
