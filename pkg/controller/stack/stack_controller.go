package stack

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/elastic/stack-operators/pkg/controller/stack/common/nodecerts"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	esclient "github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch/client"
	"github.com/elastic/stack-operators/pkg/controller/stack/kibana"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
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
var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	log            = logf.Log.WithName("stack-controller")
)

// Add creates a new Stack Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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

	return &ReconcileStack{Client: mgr.GetClient(), scheme: mgr.GetScheme(), esCa: esCa, kibanaCa: kibanaCa}, nil
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

	// Watch secrets
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
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

	esCa     *nodecerts.Ca
	kibanaCa *nodecerts.Ca

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

	internalUsers, err := r.reconcileUsers(&stack)
	if err != nil {
		return reconcile.Result{}, err
	}

	// TODO: suffix with type (es?) and trim
	clusterCAPublicSecretObjectKey := request.NamespacedName
	if err := r.esCa.ReconcilePublicCertsSecret(r, clusterCAPublicSecretObjectKey, &stack, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	res, err := r.CreateElasticsearchPods(request, internalUsers.ControllerUser)
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
	res, err = r.reconcileKibanaDeployment(&stack, internalUsers.KibanaUser, clusterCAPublicSecretObjectKey)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileService(&stack, kibana.NewService(stack))
	if err != nil {
		return res, err
	}

	res, err = r.ReconcileNodeCertificateSecrets(stack)
	if err != nil {
		return res, err
	}

	return r.DeleteElasticsearchPods(request, internalUsers.ControllerUser)
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
func NewElasticsearchClient(stack *deploymentsv1alpha1.Stack, esUser esclient.User, caPool *x509.CertPool) (*esclient.Client, error) {
	esURL, err := elasticsearch.ExternalServiceURL(stack.Name)

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		esURL = strings.Replace(esURL, "http:", "https:", 1)
	}

	return &esclient.Client{
		Endpoint: esURL,
		User:     esUser,
		HTTP: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caPool,
					// TODO: we can do better.
					InsecureSkipVerify: true,
				},
			},
		},
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
func (r *ReconcileStack) CreateElasticsearchPods(request reconcile.Request, user esclient.User) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	currentPods, err := r.GetPodList(request, elasticsearch.TypeSelector, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// TODO: suffix and trim
	elasticsearchExtraFilesSecretObjectKey := types.NamespacedName{
		Namespace: stackInstance.Namespace,
		Name:      fmt.Sprintf("%s-extrafiles", stackInstance.Name),
	}
	var elasticsearchExtraFilesSecret corev1.Secret
	if err := r.Get(
		context.TODO(),
		elasticsearchExtraFilesSecretObjectKey,
		&elasticsearchExtraFilesSecret,
	); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	} else if apierrors.IsNotFound(err) {
		// TODO: handle reconciling Data section if it already exists
		trustRootCfg := elasticsearch.TrustRootConfig{
			Trust: elasticsearch.TrustConfig{
				// the Subject Name needs to match the certificates of the nodes we want to allow to connect.
				// this needs to be kept in sync with nodecerts.buildCertificateCommonName
				SubjectName: []string{fmt.Sprintf(
					"*.node.%s.%s.es.cluster.local", stackInstance.Name, stackInstance.Namespace,
				)},
			},
		}
		trustRootCfgData, err := json.Marshal(&trustRootCfg)
		if err != nil {
			return reconcile.Result{}, err
		}

		elasticsearchExtraFilesSecret = corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      elasticsearchExtraFilesSecretObjectKey.Name,
				Namespace: elasticsearchExtraFilesSecretObjectKey.Namespace,
			},
			Data: map[string][]byte{
				"trust.yml": trustRootCfgData,
			},
		}
		controllerutil.SetControllerReference(&stackInstance, &elasticsearchExtraFilesSecret, r.scheme)

		if err := r.Create(context.TODO(), &elasticsearchExtraFilesSecret); err != nil {
			return reconcile.Result{}, err
		}
	}

	var podSpecParams = elasticsearch.BuildNewPodSpecParams(stackInstance)
	var proposedPods []corev1.Pod

	// Create any missing instances
	for i := int32(len(currentPods.Items)); i < stackInstance.Spec.Elasticsearch.NodeCount(); i++ {
		pod, err := elasticsearch.NewPod(stackInstance, user, elasticsearchExtraFilesSecretObjectKey)
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

		newPod, err := elasticsearch.NewPod(stackInstance, user, elasticsearchExtraFilesSecretObjectKey)
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
		if stackInstance.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
			log.Info(fmt.Sprintf("Ensuring that node certificate secret exists for pod %s", pod.Name))

			// create the node certificates secret for this pod, which is our promise that we will sign a CSR
			// originating from the pod after it has started and produced a CSR
			if err := nodecerts.EnsureNodeCertificateSecretExists(
				r,
				r.scheme,
				stackInstance,
				pod,
				nodecerts.LabelNodeCertificateTypeElasticsearchAll,
			); err != nil {
				return reconcile.Result{}, err
			}
		}

		if err := r.Create(context.TODO(), &pod); err != nil {
			return reconcile.Result{}, err
		}
		log.Info(common.Concat("Created Pod ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
	}

	return reconcile.Result{}, err
}

// DeleteElasticsearchPods removes running pods to match the Stack definition.
func (r *ReconcileStack) DeleteElasticsearchPods(request reconcile.Request, esUser esclient.User) (reconcile.Result, error) {
	stackInstance, err := r.GetStack(request)
	if err != nil {
		return reconcile.Result{}, err
	}

	esReachable, err := r.IsPublicServiceReady(stackInstance)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.", "iteration", atomic.LoadInt64(&r.iteration))
		return defaultRequeue, nil
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

	// create an Elasticsearch client
	certPool := x509.NewCertPool()
	certPool.AddCert(r.esCa.Cert)
	esClient, err := NewElasticsearchClient(&stackInstance, esUser, certPool)
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

func (r *ReconcileStack) reconcileKibanaDeployment(
	stack *deploymentsv1alpha1.Stack,
	user esclient.User,
	esClusterCAPublicSecretObjectKey types.NamespacedName,
) (reconcile.Result, error) {
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		CustomImageName:  stack.Spec.Kibana.Image,
		ElasticsearchUrl: elasticsearch.PublicServiceURL(stack.Name),
		User:             user,
	}

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		kibanaPodSpecParams.ElasticsearchUrl = strings.Replace(kibanaPodSpecParams.ElasticsearchUrl, "http:", "https:", 1)
	}

	kibanaPodSpec := kibana.NewPodSpec(kibanaPodSpecParams)

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		// TODO: use kibanaCa to generate cert for deployment
		// to do that, EnsureNodeCertificateSecretExists needs a deployment variant.

		esCertsVolume := elasticsearch.NewSecretVolumeWithMountPath(
			esClusterCAPublicSecretObjectKey.Name,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		kibanaPodSpec.Volumes = append(kibanaPodSpec.Volumes, esCertsVolume.Volume())

		for i, container := range kibanaPodSpec.InitContainers {
			kibanaPodSpec.InitContainers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())
		}

		for i, container := range kibanaPodSpec.Containers {
			kibanaPodSpec.Containers[i].VolumeMounts = append(container.VolumeMounts, esCertsVolume.VolumeMount())

			kibanaPodSpec.Containers[i].Env = append(
				kibanaPodSpec.Containers[i].Env,
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_CERTIFICATEAUTHORITIES",
					Value: strings.Join([]string{esCertsVolume.VolumeMount().MountPath, "ca.pem"}, "/"),
				},
				corev1.EnvVar{
					Name:  "ELASTICSEARCH_SSL_VERIFICATIONMODE",
					Value: "certificate",
				},
			)
		}
	}

	labels := kibana.NewLabelsWithStackID(common.StackID(*stack))
	deploy := NewDeployment(DeploymentParams{
		Name:      kibana.NewDeploymentName(stack.Name),
		Namespace: stack.Namespace,
		Replicas:  1,
		Selector:  labels,
		Labels:    labels,
		PodSpec:   kibanaPodSpec,
	})
	return r.ReconcileDeployment(deploy, *stack)
}

func (r *ReconcileStack) ReconcileNodeCertificateSecrets(
	stack deploymentsv1alpha1.Stack,
) (reconcile.Result, error) {
	log.Info("Reconciling node certificate secrets")

	nodeCertificateSecrets, err := r.findNodeCertificateSecrets(stack)
	if err != nil {
		return reconcile.Result{}, err
	}

	var esDiscoveryService corev1.Service
	if err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: stack.Namespace,
		Name:      elasticsearch.DiscoveryServiceName(stack.Name),
	}, &esDiscoveryService); err != nil {
		return reconcile.Result{}, err
	}
	esAllServices := []corev1.Service{esDiscoveryService}

	for _, secret := range nodeCertificateSecrets {
		log.Info("Looking at secret", "secret", secret.Name)
		// todo: error checking if label does not exist
		podName := secret.Labels[nodecerts.LabelAssociatedPod]

		var pod corev1.Pod
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: secret.Namespace, Name: podName}, &pod); err != nil {
			if !apierrors.IsNotFound(err) {
				return reconcile.Result{}, err
			}

			// give some leniency in pods showing up only after a while.
			if secret.CreationTimestamp.Add(5 * time.Minute).Before(time.Now()) {
				// if the secret has existed for too long without an associated pod, it's time to GC it
				log.Info("Unable to find pod associated with secret, GCing", "secret", secret.Name)
				if err := r.Delete(context.TODO(), &secret); err != nil {
					return reconcile.Result{}, err
				}
			} else {
				log.Info("Unable to find pod associated with secret, but secret is too young for GC", "secret", secret.Name)
			}
			continue
		}

		if pod.Status.PodIP == "" {
			log.Info("Skipping secret because associated pod has no pod ip", "secret", secret.Name)
			continue
		}

		log.Info("Secret has an associated pod that exists, will reconcile the secret", "secret", secret.Name)

		certificateType, ok := secret.Labels[nodecerts.LabelNodeCertificateType]
		if !ok {
			log.Error(errors.New("missing certificate type"), "No certificate type found", "secret", secret.Name)
			continue
		}

		switch certificateType {
		case nodecerts.LabelNodeCertificateTypeElasticsearchAll:
			if res, err := nodecerts.ReconcileNodeCertificateSecret(
				stack, secret, pod, esAllServices, r.esCa, r,
			); err != nil {
				return res, err
			}
		default:
			log.Error(
				errors.New("unsupported certificate type"),
				fmt.Sprintf("Unsupported cerificate type: %s found in %s, ignoring", certificateType, secret.Name),
			)
		}
	}

	log.Info("Node certificate secrets reconciled")
	return reconcile.Result{}, nil
}

func (r *ReconcileStack) findNodeCertificateSecrets(stack deploymentsv1alpha1.Stack) ([]corev1.Secret, error) {
	var nodeCertificateSecrets corev1.SecretList
	listOptions := client.ListOptions{
		Namespace: stack.Namespace,
		LabelSelector: labels.Set(map[string]string{
			nodecerts.LabelSecretUsage: nodecerts.LabelSecretUsageNodeCertificates,
		}).AsSelector(),
	}

	if err := r.List(context.TODO(), &listOptions, &nodeCertificateSecrets); err != nil {
		return nil, err
	}

	return nodeCertificateSecrets.Items, nil
}
