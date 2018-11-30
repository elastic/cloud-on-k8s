package elasticsearch

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	commonv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	commonversion "github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"

	elasticsearchv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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
	err = c.Watch(&source.Kind{Type: &elasticsearchv1alpha1.ElasticsearchCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// watch any pods created by Elasticsearch
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	})
	if err != nil {
		return err
	}
	// Watch services
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	})

	// Watch secrets
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &elasticsearchv1alpha1.ElasticsearchCluster{},
	})

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
func (r *ReconcileElasticsearch) Reconcile(request reconcile.Request) (result reconcile.Result, err error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()

	// Fetch the Elasticsearch instance
	es := &elasticsearchv1alpha1.ElasticsearchCluster{}
	err = r.Get(context.TODO(), request.NamespacedName, es)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	ver, err := commonversion.Parse(es.Spec.Version)
	if err != nil {
		return reconcile.Result{}, err
	}

	esVersionStrategy, err := version.LookupStrategy(*ver)
	if err != nil {
		return reconcile.Result{}, err
	}

	res, err := common.ReconcileService(r, r.scheme, support.NewDiscoveryService(*es), es)
	if err != nil {
		return res, err
	}
	res, err = common.ReconcileService(r, r.scheme, support.NewPublicService(*es), es)
	if err != nil {
		return res, err
	}

	internalUsers, err := r.reconcileUsers(es)
	if err != nil {
		return reconcile.Result{}, err
	}

	// TODO: suffix with type (es?) and trim
	clusterCAPublicSecretObjectKey := request.NamespacedName
	if err := r.esCa.ReconcilePublicCertsSecret(r, clusterCAPublicSecretObjectKey, es, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// currently we don't need any state information from the functions above, so state collections starts here
	state := NewReconcileState(*es)
	defer func() {
		_, e := r.updateStatus(&state)
		if e != nil {
			err = e
		}
	}()

	state, err = r.reconcileElasticsearchPods(state, *es, esVersionStrategy, internalUsers.ControllerUser)
	if err != nil {
		return state.Result(), err
	}

	res, err = r.ReconcileNodeCertificateSecrets(*es)
	if err != nil {
		return res, err
	}
	err = r.ReconcileSnapshotterCronJob(*es, internalUsers.ControllerUser)
	if err != nil {
		return res, err
	}
	return state.Result(), nil

}

func (r *ReconcileElasticsearch) reconcileElasticsearchPods(
	reconcileState ReconcileState,
	es elasticsearchv1alpha1.ElasticsearchCluster,
	versionStrategy version.ElasticsearchVersionStrategy,
	controllerUser esclient.User,
) (ReconcileState, error) {
	certPool := x509.NewCertPool()
	certPool.AddCert(r.esCa.Cert)
	esClient := esclient.NewElasticsearchClient(support.PublicServiceURL(es), controllerUser, certPool)

	esState, err := support.NewResourcesStateFromAPI(r, es, esClient)
	if err != nil {
		return reconcileState, err
	}

	if err := versionStrategy.VerifySupportsExistingPods(esState.CurrentPods); err != nil {
		return reconcileState, err
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
		return reconcileState, err
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
			return reconcileState, err
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
			return reconcileState, err
		}

		if err := r.Create(context.TODO(), &elasticsearchExtraFilesSecret); err != nil {
			return reconcileState, err
		}
	}

	keystoreConfig, err := r.ReconcileSnapshotCredentials(es.Spec.SnapshotRepository)
	if err != nil {
		return reconcileState, err
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
		return reconcileState, err
	}

	changes, err := support.CalculateChanges(expectedPodSpecCtxs, *esState)
	if err != nil {
		return reconcileState, err
	}

	esReachable, err := r.IsPublicServiceReady(es)
	if err != nil {
		return reconcileState, err
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

	if changes.IsEmpty() {
		// Current state matches expected state
		if esReachable {
			// Update discovery for any previously created pods that have come up (see also below in create pod)
			err := versionStrategy.UpdateDiscovery(esClient, AvailableElasticsearchNodes(esState.CurrentPods))
			if err != nil {
				log.Error(err, "Error during update discovery, continuing")
			}
		}
		reconcileState.UpdateElasticsearchState(*esState)
		return reconcileState, nil
	}

	log.Info("Going to apply the following topology changes",
		"ToAdd:", len(changes.ToAdd), "ToKeep:", len(changes.ToKeep), "ToRemove:", len(changes.ToRemove),
		"iteration", atomic.LoadInt64(&r.iteration))

	// Grow cluster with missing pods
	for _, newPodToAdd := range changes.ToAdd {
		log.Info(fmt.Sprintf("Need to add pod because of the following mismatch reasons: %v", newPodToAdd.MismatchReasons))
		err := r.CreateElasticsearchPod(es, versionStrategy, newPodToAdd.PodSpecCtx)
		if err != nil {
			return reconcileState, err
		}
		// There is no point in updating discovery settings here as the new pods will not be ready and ES will reject the
		// settings change
	}

	if !esReachable {
		// We cannot manipulate ES allocation exclude settings if the ES cluster
		// cannot be reached, hence we cannot delete pods.
		// Probably it was just created and is not ready yet.
		// Let's retry in a while.
		log.Info("ES public service not ready yet for shard migration reconciliation. Requeuing.", "iteration", atomic.LoadInt64(&r.iteration))
		reconcileState.UpdateElasticsearchPending(defaultRequeue, esState.CurrentPods)
		return reconcileState, nil
	}

	// Start migrating data away from all pods to be removed
	namesToRemove := make([]string, len(changes.ToRemove))
	for i, pod := range changes.ToRemove {
		namesToRemove[i] = pod.Name
	}
	if err = support.MigrateData(esClient, namesToRemove); err != nil {
		return reconcileState, errors.Wrap(err, "Error during migrate data")
	}

	newState := make([]corev1.Pod, len(esState.CurrentPods))
	copy(newState, esState.CurrentPods)

	// Shrink clusters by deleting deprecated pods
	for _, pod := range changes.ToRemove {
		newState = remove(newState, pod)
		preDelete := func() error {
			return versionStrategy.UpdateDiscovery(esClient, newState)
		}
		reconcileState, err = r.DeleteElasticsearchPod(reconcileState, *esState, pod, esClient, changes.ToRemove, preDelete)
		if err != nil {
			return reconcileState, err
		}
	}

	reconcileState.UpdateElasticsearchState(*esState)
	return reconcileState, nil
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
	es elasticsearchv1alpha1.ElasticsearchCluster,
	versionStrategy version.ElasticsearchVersionStrategy,
	podSpecCtx support.PodSpecContext,
) error {
	pod, err := versionStrategy.NewPod(es, podSpecCtx)
	if err != nil {
		return err
	}
	if es.Spec.FeatureFlags.Get(commonv1alpha1.FeatureFlagNodeCertificates).Enabled {
		log.Info(fmt.Sprintf("Ensuring that node certificate secret exists for pod %s", pod.Name))

		// create the node certificates secret for this pod, which is our promise that we will sign a CSR
		// originating from the pod after it has started and produced a CSR
		if err := nodecerts.EnsureNodeCertificateSecretExists(
			r,
			r.scheme,
			&es,
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

		if err := controllerutil.SetControllerReference(&es, pvc, r.scheme); err != nil {
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

	if err := controllerutil.SetControllerReference(&es, &pod, r.scheme); err != nil {
		return err
	}
	if err := r.Create(context.TODO(), &pod); err != nil {
		return err
	}
	msg := common.Concat("Created pod ", pod.Name)
	r.recorder.Event(&es, corev1.EventTypeNormal, events.EventReasonCreated, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return nil
}

// DeleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func (r *ReconcileElasticsearch) DeleteElasticsearchPod(
	reconcileState ReconcileState,
	esState support.ResourcesState,
	pod corev1.Pod,
	esClient *esclient.Client,
	allDeletions []corev1.Pod,
	preDelete func() error,
) (ReconcileState, error) {
	isMigratingData := support.IsMigratingData(esState, pod, allDeletions)
	if isMigratingData {
		log.Info(common.Concat("Migrating data, skipping deletes because of ", pod.Name), "iteration", atomic.LoadInt64(&r.iteration))
		reconcileState.UpdateElasticsearchMigrating(defaultRequeue, esState)
		return reconcileState, nil
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
			return reconcileState, err
		}

		if err := r.Delete(context.TODO(), &pvc); err != nil && !apierrors.IsNotFound(err) {
			return reconcileState, err
		}
	}

	if err := preDelete(); err != nil {
		return reconcileState, err
	}
	if err := r.Delete(context.TODO(), &pod); err != nil && !apierrors.IsNotFound(err) {
		return reconcileState, err
	}
	msg := common.Concat("Deleted pod ", pod.Name)
	reconcileState.AddEvent(corev1.EventTypeNormal, events.EventReasonDeleted, msg)
	log.Info(msg, "iteration", atomic.LoadInt64(&r.iteration))

	return reconcileState, nil
}

func (r *ReconcileElasticsearch) updateStatus(state *ReconcileState) (reconcile.Result, error) {
	log.Info("Updating status", "iteration", atomic.LoadInt64(&r.iteration))
	resource := &state.cluster
	events, cluster := state.Apply()
	for _, evt := range events {
		log.Info(fmt.Sprintf("Recording event %+v", evt))
		r.recorder.Event(resource, evt.EventType, evt.Reason, evt.Message)
	}
	if cluster == nil {
		return state.Result(), nil
	}
	return state.Result(), r.Status().Update(context.TODO(), cluster)
}

func (r *ReconcileElasticsearch) ReconcileNodeCertificateSecrets(
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
