package elasticsearch

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	commonversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
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

var (
	defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
	log            = logf.Log.WithName("elasticsearch-controller")
)

// Add creates a new Elasticsearch Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this elasticsearch.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	reconciler, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, reconciler)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	esCa, err := nodecerts.NewSelfSignedCa("elasticsearch-controller")
	if err != nil {
		return nil, err
	}

	return &ReconcileElasticsearch{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("elasticsearch-controller"),

		esCa: esCa,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("elasticsearch-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Elasticsearch
	if err := c.Watch(
		&source.Kind{Type: &elasticsearchv1alpha1.ElasticsearchCluster{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}

	// watch any pods created by Elasticsearch
	if err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	}); err != nil {
		return err
	}

	// Watch services
	if err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	}); err != nil {
		return err
	}

	// Watch secrets
	if err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	}); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileElasticsearch{}

// ReconcileElasticsearch reconciles a Elasticsearch object
type ReconcileElasticsearch struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder

	esCa *nodecerts.Ca

	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}

// Reconcile reads that state of the cluster for a Elasticsearch object and makes changes based on the state read and
// what is in the Elasticsearch.Spec
//
// Automatically generate RBAC rules:
// +kubebuilder:rbac:groups=,resources=pods;endpoints;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=elasticsearch.k8s.elastic.co,resources=elasticsearchclusters;elasticsearchclusters/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the Elasticsearch instance
	es := &elasticsearchv1alpha1.ElasticsearchCluster{}
	err := r.Get(context.TODO(), request.NamespacedName, es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	state := NewReconcileState(*es)
	results := r.internalReconcile(&state)
	err = r.updateStatus(&state)
	return results.WithError(err).Aggregate()
}

func (r *ReconcileElasticsearch) internalReconcile(state *ReconcileState) *ReconcileResults {
	es := &state.cluster
	results := &ReconcileResults{}
	ver, err := commonversion.Parse(es.Spec.Version)
	if err != nil {
		return results.WithError(err)
	}

	esVersionStrategy, err := version.LookupStrategy(*ver)
	if err != nil {
		return results.WithError(err)
	}

	res, err := common.ReconcileService(r, r.scheme, support.NewDiscoveryService(*es), es)
	if err != nil {
		return results.WithError(err).WithResult(res)
	}

	res, err = common.ReconcileService(r, r.scheme, support.NewPublicService(*es), es)
	if err != nil {
		return results.WithError(err).WithResult(res)
	}

	// TODO: suffix with type (es?) and trim
	clusterCAPublicSecretObjectKey := types.NamespacedName{Namespace: es.Namespace, Name: es.Name}
	if err := r.esCa.ReconcilePublicCertsSecret(r, clusterCAPublicSecretObjectKey, es, r.scheme); err != nil {
		return results.WithError(err)
	}

	internalUsers, err := r.reconcileUsers(es)
	if err != nil {
		return results.WithError(err)
	}

	// recoverable reconcile steps start here. In case of error we record the error and continue
	results.Apply("reconcileNodeCertificateSecrets", func() (reconcile.Result, error) {
		return r.reconcileNodeCertificateSecrets(*es)
	})

	results.Apply("reconcileElasticsearchPods", func() (reconcile.Result, error) {
		return r.reconcileElasticsearchPods(state, *es, esVersionStrategy, internalUsers.ControllerUser)
	})

	err = r.ReconcileSnapshotterCronJob(*es, internalUsers.ControllerUser)
	return results.WithError(err)
}

func (r *ReconcileElasticsearch) reconcileElasticsearchPods(
	reconcileState *ReconcileState,
	es elasticsearchv1alpha1.ElasticsearchCluster,
	versionStrategy version.ElasticsearchVersionStrategy,
	controllerUser esclient.User,
) (reconcile.Result, error) {
	certPool := x509.NewCertPool()
	certPool.AddCert(r.esCa.Cert)
	esClient := esclient.NewElasticsearchClient(support.PublicServiceURL(es), controllerUser, certPool)

	resourcesState, err := support.NewResourcesStateFromAPI(r, es)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := versionStrategy.VerifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
		return reconcile.Result{}, err
	}

	observedState := support.NewObservedState(esClient)

	// always update the elasticsearch state bits
	if observedState.ClusterState != nil && observedState.ClusterHealth != nil {
		reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	}

	// TODO: suffix and trim
	elasticsearchExtraFilesSecretObjectKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      fmt.Sprintf("%s-extrafiles", es.Name),
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
		trustRootCfg := support.TrustRootConfig{
			Trust: support.TrustConfig{
				// the Subject Name needs to match the certificates of the nodes we want to allow to connect.
				// this needs to be kept in sync with nodecerts.buildCertificateCommonName
				SubjectName: []string{fmt.Sprintf(
					"*.node.%s.%s.es.cluster.local", es.Name, es.Namespace,
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

		err = controllerutil.SetControllerReference(&es, &elasticsearchExtraFilesSecret, r.scheme)
		if err != nil {
			return reconcile.Result{}, err
		}

		if err := r.Create(context.TODO(), &elasticsearchExtraFilesSecret); err != nil {
			return reconcile.Result{}, err
		}
	}

	keystoreConfig, err := r.ReconcileSnapshotCredentials(es.Spec.SnapshotRepository)
	if err != nil {
		return reconcile.Result{}, err
	}

	podSpecParamsTemplate := support.NewPodSpecParams{
		ExtraFilesRef:  elasticsearchExtraFilesSecretObjectKey,
		KeystoreConfig: keystoreConfig,
		ProbeUser:      controllerUser,
	}

	expectedPodSpecCtxs, err := versionStrategy.ExpectedPodSpecs(
		es,
		podSpecParamsTemplate,
	)

	if err != nil {
		return reconcile.Result{}, err
	}

	changes, err := mutation.CalculateChanges(expectedPodSpecCtxs, *resourcesState, func(ctx support.PodSpecContext) (corev1.Pod, error) {
		return versionStrategy.NewPod(es, ctx)
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	log.Info(
		"Going to apply the following topology changes",
		"ToCreate:", len(changes.ToCreate), "ToKeep:", len(changes.ToKeep), "ToDelete:", len(changes.ToDelete),
		"iteration", atomic.LoadInt64(&r.iteration),
	)

	esReachable, err := r.IsPublicServiceReady(es)
	if err != nil {
		return reconcile.Result{}, err
	}

	if esReachable { // TODO this needs to happen outside of reconcileElasticsearchPods pending refactoring
		err = snapshots.EnsureSnapshotRepository(context.TODO(), esClient, es.Spec.SnapshotRepository)
		if err != nil {
			// TODO decide should this be a reason to stop this reconciliation loop?
			msg := "Could not ensure snapshot repository"
			reconcileState.AddEvent(corev1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg, "iteration", atomic.LoadInt64(&r.iteration))
		}
	}

	// build the current state of the pods.
	podsState := mutation.NewPodsState(*resourcesState, observedState)

	// figure out what changes we can perform right now
	performableChanges, err := mutation.CalculatePerformableChanges(
		es.Spec.UpdateStrategy,
		&changes,
		podsState,
	)
	if err != nil {
		return reconcile.Result{}, err
	}

	log.Info(
		"Calculated performable changes",
		"schedule_for_creation_count", len(performableChanges.ToCreate),
		"schedule_for_deletion_count", len(performableChanges.ToDelete),
	)

	for _, change := range performableChanges.ToCreate {
		if err := r.CreateElasticsearchPod(reconcileState, versionStrategy, change.Pod, change.PodSpecCtx); err != nil {
			return reconcile.Result{}, err
		}
	}

	// reconcile node certificates since we might have new pods (or existing pods that needs a refresh)
	if res, err := r.reconcileNodeCertificateSecrets(es); err != nil {
		return res, err
	}

	if !changes.HasChanges() {
		// Current state matches expected state
		if !esReachable {
			// es not yet reachable, let's try again later.
			return defaultRequeue, nil
		}

		// Update discovery for any previously created pods that have come up (see also below in create pod)
		if err := versionStrategy.UpdateDiscovery(
			esClient,
			AvailableElasticsearchNodes(resourcesState.CurrentPods),
		); err != nil {
			log.Error(err, "Error during update discovery after having no changes, requeuing.")
			return defaultRequeue, nil
		}

		reconcileState.UpdateElasticsearchOperational(*resourcesState, observedState)
		return reconcile.Result{}, nil
	}

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.")

		reconcileState.UpdateElasticsearchPending(resourcesState.CurrentPods)

		return defaultRequeue, nil
	}

	// Start migrating data away from all pods to be deleted
	leavingNodeNames := support.PodListToNames(performableChanges.ToDelete)
	if err = support.MigrateData(esClient, leavingNodeNames); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "error during migrate data")
	}

	newState := make([]corev1.Pod, len(resourcesState.CurrentPods))
	copy(newState, resourcesState.CurrentPods)

	results := ReconcileResults{}
	// Shrink clusters by deleting deprecated pods
	for _, pod := range performableChanges.ToDelete {
		newState = remove(newState, pod)
		preDelete := func() error {
			return versionStrategy.UpdateDiscovery(esClient, newState)
		}
		result, err := r.DeleteElasticsearchPod(
			reconcileState,
			*resourcesState,
			observedState,
			pod,
			esClient,
			performableChanges.ToDelete,
			preDelete,
		)
		if err != nil {
			return result, err
		}
		results.WithResult(result)
	}
	if changes.HasChanges() && !performableChanges.HasChanges() {
		// if there are changes we'd like to perform, but none that were performable, we try again later
		results.WithResult(defaultRequeue)
	}

	reconcileState.UpdateElasticsearchState(*resourcesState, observedState)
	return results.Aggregate()
}

func remove(pods []corev1.Pod, pod corev1.Pod) []corev1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}

// CreateElasticsearchPod creates the given elasticsearch pod
func (r *ReconcileElasticsearch) CreateElasticsearchPod(
	state *ReconcileState,
	versionStrategy version.ElasticsearchVersionStrategy,
	pod corev1.Pod,
	podSpecCtx support.PodSpecContext,
) error {
	// create the node certificates secret for this pod, which is our promise that we will sign a CSR
	// originating from the pod after it has started and produced a CSR
	log.Info(fmt.Sprintf("Ensuring that node certificate secret exists for pod %s", pod.Name))
	nodeCertificatesSecret, err := nodecerts.EnsureNodeCertificateSecretExists(
		r,
		r.scheme,
		&state.cluster,
		pod,
		nodecerts.LabelNodeCertificateTypeElasticsearchAll,
	)
	if err != nil {
		return err
	}

	// we finally have the node certificates secret made, so we can inject the secret volume into the pod
	nodeCertificatesSecretVolume := support.NewSecretVolumeWithMountPath(
		nodeCertificatesSecret.Name,
		support.NodeCertificatesSecretVolumeName,
		support.NodeCertificatesSecretVolumeMountPath,
	)
	// add the node certificates volume to volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, nodeCertificatesSecretVolume.Volume())

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

		if err := controllerutil.SetControllerReference(&state.cluster, pvc, r.scheme); err != nil {
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

	if err := controllerutil.SetControllerReference(&state.cluster, &pod, r.scheme); err != nil {
		return err
	}
	if err := r.Create(context.TODO(), &pod); err != nil {
		return err
	}
	msg := common.Concat("Created pod ", pod.Name)
	state.AddEvent(corev1.EventTypeNormal, events.EventReasonCreated, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return nil
}

// DeleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func (r *ReconcileElasticsearch) DeleteElasticsearchPod(
	reconcileState *ReconcileState,
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
	pod corev1.Pod,
	esClient *esclient.Client,
	allDeletions []corev1.Pod,
	preDelete func() error,
) (reconcile.Result, error) {
	isMigratingData := support.IsMigratingData(observedState, pod, allDeletions)
	if isMigratingData {
		log.Info(common.Concat("Migrating data, skipping deletes because of ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		reconcileState.UpdateElasticsearchMigrating(resourcesState, observedState)
		return defaultRequeue, nil
	}

	// delete all PVCs associated with this pod
	// TODO: perhaps this is better to reconcile after the fact?
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			continue
		}

		// TODO: perhaps not assuming all PVCs will be managed by us? and maybe we should not categorically delete?
		pvc, err := resourcesState.FindPVCByName(volume.PersistentVolumeClaim.ClaimName)
		if err != nil {
			return reconcile.Result{}, err
		}

		if err := r.Delete(context.TODO(), &pvc); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	}

	if err := preDelete(); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.Delete(context.TODO(), &pod); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	msg := common.Concat("Deleted pod ", pod.Name)
	reconcileState.AddEvent(corev1.EventTypeNormal, events.EventReasonDeleted, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return reconcile.Result{}, nil
}

func (r *ReconcileElasticsearch) updateStatus(state *ReconcileState) error {
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	resource := &state.cluster
	events, cluster := state.Apply()
	for _, evt := range events {
		log.Info(fmt.Sprintf("Recording event %+v", evt))
		r.recorder.Event(resource, evt.EventType, evt.Reason, evt.Message)
	}
	if cluster == nil {
		return nil
	}
	return r.Status().Update(context.TODO(), cluster)
}

func (r *ReconcileElasticsearch) reconcileNodeCertificateSecrets(
	es elasticsearchv1alpha1.ElasticsearchCluster,
) (reconcile.Result, error) {
	log.Info("Reconciling node certificate secrets")

	nodeCertificateSecrets, err := r.findNodeCertificateSecrets(es)
	if err != nil {
		return reconcile.Result{}, err
	}

	var esDiscoveryService corev1.Service
	if err := r.Get(context.TODO(), types.NamespacedName{
		Namespace: es.Namespace,
		Name:      support.DiscoveryServiceName(es.Name),
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
				secret, pod, es.Name, es.Namespace, esAllServices, r.esCa, r,
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

func (r *ReconcileElasticsearch) findNodeCertificateSecrets(es elasticsearchv1alpha1.ElasticsearchCluster) ([]corev1.Secret, error) {
	var nodeCertificateSecrets corev1.SecretList
	listOptions := client.ListOptions{
		Namespace: es.Namespace,
		LabelSelector: labels.Set(map[string]string{
			nodecerts.LabelSecretUsage: nodecerts.LabelSecretUsageNodeCertificates,
		}).AsSelector(),
	}

	if err := r.List(context.TODO(), &listOptions, &nodeCertificateSecrets); err != nil {
		return nil, err
	}

	return nodeCertificateSecrets.Items, nil
}

// IsPublicServiceReady checks if Elasticsearch public service is ready,
// so that the ES cluster can respond to HTTP requests.
// Here we just check that the service has endpoints to route requests to.
func (r *ReconcileElasticsearch) IsPublicServiceReady(es elasticsearchv1alpha1.ElasticsearchCluster) (bool, error) {
	endpoints := corev1.Endpoints{}
	publicService := support.NewPublicService(es).ObjectMeta
	namespacedName := types.NamespacedName{Namespace: publicService.Namespace, Name: publicService.Name}
	err := r.Get(context.TODO(), namespacedName, &endpoints)
	if err != nil {
		return false, err
	}
	for _, subs := range endpoints.Subsets {
		if len(subs.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}
