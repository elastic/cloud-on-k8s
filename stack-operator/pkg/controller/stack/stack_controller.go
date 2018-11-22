package stack

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/snapshots"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/kibana"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/state"
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

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

const (
	caChecksumLabelName = "kibana.stack.k8s.elastic.co/ca-file-checksum"
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
	scheme   *runtime.Scheme
	recorder record.EventRecorder

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
// +kubebuilder:rbac:groups=,resources=pods;endpoints;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deployments.k8s.elastic.co,resources=stacks;stacks/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileStack) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// To support concurrent runs.
	atomic.AddInt64(&r.iteration, 1)

	stack, err := r.GetStack(request.NamespacedName)
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

	res, err := r.reconcileService(&stack, elasticsearch.NewDiscoveryService(stack))
	if err != nil {
		return res, err
	}
	res, err = r.reconcileService(&stack, elasticsearch.NewPublicService(stack))
	if err != nil {
		return res, err
	}

	// currently we don't need any state information from the functions above, so state collections starts here
	state := state.NewReconcileState(request, &stack)

	state, err = r.reconcileElasticsearchPods(state, stack, internalUsers.ControllerUser)
	if err != nil {
		return state.Result, err
	}

	state, err = r.reconcileKibanaDeployment(state, &stack, internalUsers.KibanaUser, clusterCAPublicSecretObjectKey)
	if err != nil {
		return state.Result, err
	}
	res, err = r.reconcileService(&stack, kibana.NewService(stack))
	if err != nil {
		return res, err
	}

	res, err = r.ReconcileNodeCertificateSecrets(stack)
	if err != nil {
		return res, err
	}
	err = r.ReconcileSnapshotterCronJob(stack, internalUsers.ControllerUser)
	if err != nil {
		return res, err
	}
	return r.updateStatus(state)
}

// GetStack obtains the stack from the backend kubernetes API.
func (r *ReconcileStack) GetStack(name types.NamespacedName) (deploymentsv1alpha1.Stack, error) {
	var stackInstance deploymentsv1alpha1.Stack
	if err := r.Get(context.TODO(), name, &stackInstance); err != nil {
		return stackInstance, err
	}
	return stackInstance, nil
}

func (r *ReconcileStack) reconcileElasticsearchPods(
	state state.ReconcileState,
	stack deploymentsv1alpha1.Stack,
	controllerUser esclient.User,
) (state.ReconcileState, error) {
	esState, err := elasticsearch.NewResourcesStateFromAPI(r, stack)
	if err != nil {
		return state, err
	}

	// TODO: suffix and trim
	elasticsearchExtraFilesSecretObjectKey := types.NamespacedName{
		Namespace: stack.Namespace,
		Name:      fmt.Sprintf("%s-extrafiles", stack.Name),
	}
	var elasticsearchExtraFilesSecret corev1.Secret
	if err := r.Get(
		context.TODO(),
		elasticsearchExtraFilesSecretObjectKey,
		&elasticsearchExtraFilesSecret,
	); err != nil && !apierrors.IsNotFound(err) {
		return state, err
	} else if apierrors.IsNotFound(err) {
		// TODO: handle reconciling Data section if it already exists
		trustRootCfg := elasticsearch.TrustRootConfig{
			Trust: elasticsearch.TrustConfig{
				// the Subject Name needs to match the certificates of the nodes we want to allow to connect.
				// this needs to be kept in sync with nodecerts.buildCertificateCommonName
				SubjectName: []string{fmt.Sprintf(
					"*.node.%s.%s.es.cluster.local", stack.Name, stack.Namespace,
				)},
			},
		}
		trustRootCfgData, err := json.Marshal(&trustRootCfg)
		if err != nil {
			return state, err
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

		err = controllerutil.SetControllerReference(&stack, &elasticsearchExtraFilesSecret, r.scheme)
		if err != nil {
			return state, err
		}

		if err := r.Create(context.TODO(), &elasticsearchExtraFilesSecret); err != nil {
			return state, err
		}
	}

	keystoreConfig, err := r.ReconcileSnapshotCredentials(stack.Spec.Elasticsearch.SnapshotRepository)
	if err != nil {
		return state, err
	}

	nonSpecParams := elasticsearch.NewPodExtraParams{
		ExtraFilesRef:  elasticsearchExtraFilesSecretObjectKey,
		KeystoreConfig: keystoreConfig,
	}

	expectedPodSpecCtxs, err := elasticsearch.CreateExpectedPodSpecs(
		stack, controllerUser, nonSpecParams,
	)

	if err != nil {
		return state, err
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(r.esCa.Cert)
	esClient := esclient.NewElasticsearchClient(elasticsearch.PublicServiceURL(stack), controllerUser, certPool)

	changes, err := elasticsearch.CalculateChanges(expectedPodSpecCtxs, *esState)
	if err != nil {
		return state, err
	}

	esReachable, err := r.IsPublicServiceReady(stack)
	if err != nil {
		return state, err
	}

	if esReachable { // TODO this needs to happen outside of reconcileElasticsearchPods pending refactoring
		err = snapshots.EnsureSnapshotRepository(context.TODO(), esClient, stack.Spec.Elasticsearch.SnapshotRepository)
		if err != nil {
			// TODO decide should this be a reason to stop this reconciliation loop?
			msg := "Could not ensure snapshot repository"
			r.recorder.Event(&stack, corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg, "iteration", atomic.LoadInt64(&r.iteration))
		}
	}

	if !changes.ShouldMigrate() {
		// Current state matches expected state
		if err := state.UpdateElasticsearchState(*esState, esClient, esReachable); err != nil {
			return state, err
		}
		return state, nil
	}

	log.Info("Going to apply the following topology changes",
		"ToAdd:", len(changes.ToAdd), "ToKeep:", len(changes.ToKeep), "ToRemove:", len(changes.ToRemove),
		"iteration", atomic.LoadInt64(&r.iteration))

	// Grow cluster with missing pods
	for _, newPod := range changes.ToAdd {
		log.Info(fmt.Sprintf("Need to add pod because of the following mismatch reasons: %v", newPod.MismatchReasons))
		if err := r.CreateElasticsearchPod(stack, newPod.PodSpecCtx); err != nil {
			return state, err
		}
	}

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.", "iteration", atomic.LoadInt64(&r.iteration))
		state.UpdateElasticsearchPending(defaultRequeue, esState.CurrentPods)
		return state, nil
	}

	// Start migrating data away from all pods to be removed
	namesToRemove := make([]string, len(changes.ToRemove))
	for i, pod := range changes.ToRemove {
		namesToRemove[i] = pod.Name
	}
	if err = elasticsearch.MigrateData(esClient, namesToRemove); err != nil {
		return state, errors.Wrap(err, "Error during migrate data")
	}

	// Shrink clusters by deleting deprecated pods
	for _, pod := range changes.ToRemove {
		state, err = r.DeleteElasticsearchPod(state, *esState, pod, esClient, changes.ToRemove)
		if err != nil {
			return state, err
		}
	}

	if err := state.UpdateElasticsearchState(*esState, esClient, esReachable); err != nil {
		return state, err
	}

	return state, nil
}

// CreateElasticsearchPod creates the given elasticsearch pod
func (r *ReconcileStack) CreateElasticsearchPod(
	stack deploymentsv1alpha1.Stack,
	podSpecCtx elasticsearch.PodSpecContext,
) error {
	pod, err := elasticsearch.NewPod(stack, podSpecCtx)
	if err != nil {
		return err
	}
	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info(fmt.Sprintf("Ensuring that node certificate secret exists for pod %s", pod.Name))

		// create the node certificates secret for this pod, which is our promise that we will sign a CSR
		// originating from the pod after it has started and produced a CSR
		if err := nodecerts.EnsureNodeCertificateSecretExists(
			r,
			r.scheme,
			stack,
			pod,
			nodecerts.LabelNodeCertificateTypeElasticsearchAll,
		); err != nil {
			return err
		}
	}

	// when can we re-use a v1.PersistentVolumeClaim?
	// - It is the same size, storageclass etc, or resizable as such
	// 		(https://kubernetes.io/docs/concepts/storage/persistent-volumes/#expanding-persistent-volumes-claims)
	// - If a local volume: when we know it's going to the same node
	//   - How can we tell?
	//     - Only guaranteed if a required node affinity specifies a specific, singular node.
	//       - Usually they are more generic, yielding a range of possible target nodes
	// - If an EBS and non-regional PDs (GCP) volume: when we know it's going to the same AZ:
	// 	 - How can we tell?
	//     - Only guaranteed if a required node affinity specifies a specific availability zone
	//       - Often
	//     - This is /hard/
	// - Other persistent
	//
	// - Limitations
	//   - Node-specific volume limits: https://kubernetes.io/docs/concepts/storage/storage-limits/
	//
	// How to technically re-use a volume:
	// - Re-use the same name for the PVC.
	//   - E.g, List PVCs, if a PVC we want to use exist

	for _, claimTemplate := range podSpecCtx.TopologySpec.VolumeClaimTemplates {
		pvc := claimTemplate.DeepCopy()
		// generate unique name for this pvc.
		// TODO: this may become too long?
		pvc.Name = pod.Name + "-" + claimTemplate.Name
		pvc.Namespace = pod.Namespace

		// we re-use the labels and annotation from the associated pod, which is used to select these PVCs when
		// reflecting state from K8s.
		pvc.Labels = pod.Labels
		pvc.Annotations = pod.Annotations
		// TODO: add more labels or annotations?

		log.Info(fmt.Sprintf("Creating PVC for pod %s: %s", pod.Name, pvc.Name))

		if err := controllerutil.SetControllerReference(&stack, pvc, r.scheme); err != nil {
			return err
		}

		if err := r.Create(context.TODO(), pvc); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}

		// delete the volume with the same name as our claim template, we will add the expected one later
		for i, volume := range pod.Spec.Volumes {
			if volume.Name == claimTemplate.Name {
				pod.Spec.Volumes = append(pod.Spec.Volumes[:i], pod.Spec.Volumes[i+1:]...)
				break
			}
		}

		// append our PVC to the list of volumes
		pod.Spec.Volumes = append(
			pod.Spec.Volumes,
			corev1.Volume{
				Name: claimTemplate.Name,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
						// TODO: support read only pvcs
					},
				},
			},
		)
	}

	if err := controllerutil.SetControllerReference(&stack, &pod, r.scheme); err != nil {
		return err
	}
	if err := r.Create(context.TODO(), &pod); err != nil {
		return err
	}
	msg := common.Concat("Created pod ", pod.Name)
	r.recorder.Event(&stack, corev1.EventTypeNormal, events.EventReasonCreated, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return nil
}

// DeleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func (r *ReconcileStack) DeleteElasticsearchPod(
	state state.ReconcileState,
	esState elasticsearch.ResourcesState,
	pod corev1.Pod,
	esClient *esclient.Client,
	allDeletions []corev1.Pod,
) (state.ReconcileState, error) {
	isMigratingData, err := elasticsearch.IsMigratingData(esClient, pod, allDeletions)
	if err != nil {
		return state, err
	}
	if isMigratingData {
		r.recorder.Event(state.Stack, corev1.EventTypeNormal, events.EventReasonDelayed, "Requested topology change delayed by data migration")
		log.Info(common.Concat("Migrating data, skipping deletes because of ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		return state, state.UpdateElasticsearchMigrating(defaultRequeue, esState, esClient)
	}

	// delete all PVCs associated with this pod
	// TODO: perhaps this is better to reconcile after the fact?
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		// TODO: perhaps not assuming all PVCs will be managed by us? and maybe we should not categorically delete?
		pvc, err := esState.FindPVCByName(volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return state, err
		}

		if err := r.Delete(context.TODO(), &pvc); err != nil && !apierrors.IsNotFound(err) {
			return state, err
		}
	}

	if err := r.Delete(context.TODO(), &pod); err != nil && !apierrors.IsNotFound(err) {
		return state, err
	}
	msg := common.Concat("Deleted Pod ", pod.Name)
	r.recorder.Event(state.Stack, corev1.EventTypeNormal, events.EventReasonDeleted, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return state, nil
}

func (r *ReconcileStack) reconcileKibanaDeployment(
	state state.ReconcileState,
	stack *deploymentsv1alpha1.Stack,
	user esclient.User,
	esClusterCAPublicSecretObjectKey types.NamespacedName,
) (state.ReconcileState, error) {
	kibanaPodSpecParams := kibana.PodSpecParams{
		Version:          stack.Spec.Version,
		CustomImageName:  stack.Spec.Kibana.Image,
		ElasticsearchUrl: elasticsearch.PublicServiceURL(*stack),
		User:             user,
	}

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		kibanaPodSpecParams.ElasticsearchUrl = strings.Replace(kibanaPodSpecParams.ElasticsearchUrl, "http:", "https:", 1)
	}

	kibanaPodSpec := kibana.NewPodSpec(kibanaPodSpecParams)
	labels := kibana.NewLabelsWithStackID(common.StackID(*stack))
	podLabels := kibana.NewLabelsWithStackID(common.StackID(*stack))

	if stack.Spec.FeatureFlags.Get(deploymentsv1alpha1.FeatureFlagNodeCertificates).Enabled {
		// TODO: use kibanaCa to generate cert for deployment
		// to do that, EnsureNodeCertificateSecretExists needs a deployment variant.

		esCertsVolume := elasticsearch.NewSecretVolumeWithMountPath(
			esClusterCAPublicSecretObjectKey.Name,
			"elasticsearch-certs",
			"/usr/share/kibana/config/elasticsearch-certs",
		)

		// build a checksum of the ca file used by ES, which we can use to cause the Deployment to roll the Kibana
		// instances in the deployment when the ca file contents change. this is done because Kibana do not support
		// updating the ca.pem file contents without restarting the process.
		caChecksum := ""
		var esPublicCASecret corev1.Secret
		if err := r.Get(context.TODO(), esClusterCAPublicSecretObjectKey, &esPublicCASecret); err != nil {
			return state, err
		}
		if capem, ok := esPublicCASecret.Data[nodecerts.SecretCAKey]; ok {
			caChecksum = fmt.Sprintf("%x", sha256.Sum224(capem))
		}
		// we add the checksum to a label for the deployment and its pods (the important bit is that the pod template
		// changes, which will trigger a rolling update)
		podLabels[caChecksumLabelName] = caChecksum

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

	deploy := NewDeployment(DeploymentParams{
		Name:      kibana.NewDeploymentName(stack.Name),
		Namespace: stack.Namespace,
		Replicas:  stack.Spec.Kibana.NodeCount,
		Selector:  labels,
		Labels:    labels,
		PodLabels: podLabels,
		PodSpec:   kibanaPodSpec,
	})
	result, err := r.ReconcileDeployment(deploy, *stack)
	if err != nil {
		return state, err
	}
	state.UpdateKibanaState(result)
	return state, nil
}

func (r *ReconcileStack) updateStatus(state state.ReconcileState) (reconcile.Result, error) {
	current, err := r.GetStack(state.Request.NamespacedName)
	if err != nil {
		return state.Result, err
	}
	if reflect.DeepEqual(current.Status, state.Stack.Status) {
		return state.Result, nil
	}
	if state.Stack.Status.Elasticsearch.IsDegraded(current.Status.Elasticsearch) {
		r.recorder.Event(&current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Elasticsearch health degraded")
	}
	if state.Stack.Status.Kibana.IsDegraded(current.Status.Kibana) {
		r.recorder.Event(&current, corev1.EventTypeWarning, events.EventReasonUnhealthy, "Kibana health degraded")
	}
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	return state.Result, r.Status().Update(context.TODO(), state.Stack)
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
