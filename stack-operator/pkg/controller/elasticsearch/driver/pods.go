package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	esclient "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/configmaps"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/mutation"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelpers"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/services"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/snapshots"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/pkg/errors"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var defaultRequeue = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

func (d *strategyDriver) reconcileElasticsearchPods(
	c client.Client,
	scheme *runtime.Scheme,
	ca *nodecerts.Ca,
	es v1alpha1.ElasticsearchCluster,
	publicService v1.Service,
	esClient *esclient.Client,
	state mutation.PodsState,
	reconcileState *reconcilehelpers.ReconcileState,
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
	podsState mutation.PodsState,
	versionStrategy version.ElasticsearchVersionStrategy,
	controllerUser esclient.User,
) (reconcile.Result, error) {
	if err := versionStrategy.VerifySupportsExistingPods(resourcesState.CurrentPods); err != nil {
		return reconcile.Result{}, err
	}

	// TODO: suffix and trim
	elasticsearchExtraFilesSecretObjectKey := types.NamespacedName{
		Namespace: es.Namespace,
		Name:      fmt.Sprintf("%s-extrafiles", es.Name),
	}
	var elasticsearchExtraFilesSecret v1.Secret
	if err := c.Get(
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

		elasticsearchExtraFilesSecret = v1.Secret{
			ObjectMeta: k8s.ToObjectMeta(elasticsearchExtraFilesSecretObjectKey),
			Data: map[string][]byte{
				"trust.yml": trustRootCfgData,
			},
		}

		err = controllerutil.SetControllerReference(&es, &elasticsearchExtraFilesSecret, scheme)
		if err != nil {
			return reconcile.Result{}, err
		}

		if err := c.Create(context.TODO(), &elasticsearchExtraFilesSecret); err != nil {
			return reconcile.Result{}, err
		}
	}

	keystoreConfig, err := snapshots.ReconcileSnapshotCredentials(c, es.Spec.SnapshotRepository)
	if err != nil {
		return reconcile.Result{}, err
	}

	expectedConfigMap := versionStrategy.ExpectedConfigMap(es)
	err = configmaps.ReconcileConfigMap(c, scheme, es, expectedConfigMap)
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

	changes, err := mutation.CalculateChanges(
		expectedPodSpecCtxs,
		resourcesState,
		func(ctx support.PodSpecContext) (v1.Pod, error) {
			return versionStrategy.NewPod(es, ctx)
		},
	)
	if err != nil {
		return reconcile.Result{}, err
	}

	log.Info(
		"Going to apply the following topology changes",
		"ToCreate:", len(changes.ToCreate),
		"ToKeep:", len(changes.ToKeep),
		"ToDelete:", len(changes.ToDelete),
	)

	esReachable, err := services.IsServiceReady(c, publicService)
	if err != nil {
		return reconcile.Result{}, err
	}

	if esReachable { // TODO this needs to happen outside of reconcileElasticsearchPods pending refactoring
		err = snapshots.EnsureSnapshotRepository(context.TODO(), esClient, es.Spec.SnapshotRepository)
		if err != nil {
			// TODO decide should this be a reason to stop this reconciliation loop?
			msg := "Could not ensure snapshot repository"
			reconcileState.AddEvent(v1.EventTypeWarning, events.EventReasonUnexpected, msg)
			log.Error(err, msg)
		}
	}

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
		if err := CreateElasticsearchPod(c, scheme, es, reconcileState, change.Pod, change.PodSpecCtx); err != nil {
			return reconcile.Result{}, err
		}
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
			reconcilehelpers.AvailableElasticsearchNodes(resourcesState.CurrentPods),
		); err != nil {
			log.Error(err, "Error during update discovery after having no changes, requeuing.")
			return defaultRequeue, nil
		}

		reconcileState.UpdateElasticsearchOperational(resourcesState, observedState)
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

	newState := make([]v1.Pod, len(resourcesState.CurrentPods))
	copy(newState, resourcesState.CurrentPods)

	results := reconcilehelpers.ReconcileResults{}
	// Shrink clusters by deleting deprecated pods
	for _, pod := range performableChanges.ToDelete {
		newState = remove(newState, pod)
		preDelete := func() error {
			return versionStrategy.UpdateDiscovery(esClient, newState)
		}
		result, err := DeleteElasticsearchPod(
			c,
			reconcileState,
			resourcesState,
			observedState,
			pod,
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

	reconcileState.UpdateElasticsearchState(resourcesState, observedState)
	return results.Aggregate()
}

func remove(pods []v1.Pod, pod v1.Pod) []v1.Pod {
	for i, p := range pods {
		if p.Name == pod.Name {
			return append(pods[:i], pods[i+1:]...)
		}
	}
	return pods
}

// CreateElasticsearchPod creates the given elasticsearch pod
func CreateElasticsearchPod(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	state *reconcilehelpers.ReconcileState,
	pod v1.Pod,
	podSpecCtx support.PodSpecContext,
) error {
	// create the node certificates secret for this pod, which is our promise that we will sign a CSR
	// originating from the pod after it has started and produced a CSR
	log.Info(fmt.Sprintf("Ensuring that node certificate secret exists for pod %s", pod.Name))
	nodeCertificatesSecret, err := nodecerts.EnsureNodeCertificateSecretExists(
		c,
		scheme,
		&es,
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

	// when can we re-use a metav1.PersistentVolumeClaim?
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

		if err := controllerutil.SetControllerReference(&es, pvc, scheme); err != nil {
			return err
		}

		if err := c.Create(context.TODO(), pvc); err != nil && !apierrors.IsAlreadyExists(err) {
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
			v1.Volume{
				Name: claimTemplate.Name,
				VolumeSource: v1.VolumeSource{
					PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
						ClaimName: pvc.Name,
						// TODO: support read only pvcs
					},
				},
			},
		)
	}

	if err := controllerutil.SetControllerReference(&es, &pod, scheme); err != nil {
		return err
	}
	if err := c.Create(context.TODO(), &pod); err != nil {
		return err
	}
	state.AddEvent(v1.EventTypeNormal, events.EventReasonCreated, common.Concat("Created pod ", pod.Name))
	log.Info("Created pod", "name", pod.Name, "namespace", pod.Namespace)

	return nil
}

// DeleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func DeleteElasticsearchPod(
	c client.Client,
	reconcileState *reconcilehelpers.ReconcileState,
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
	pod v1.Pod,
	allDeletions []v1.Pod,
	preDelete func() error,
) (reconcile.Result, error) {
	isMigratingData := support.IsMigratingData(observedState, pod, allDeletions)
	if isMigratingData {
		log.Info(common.Concat("Migrating data, skipping deletes because of ", pod.Name))
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

		if err := c.Delete(context.TODO(), &pvc); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	}

	if err := preDelete(); err != nil {
		return reconcile.Result{}, err
	}
	if err := c.Delete(context.TODO(), &pod); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	reconcileState.AddEvent(
		v1.EventTypeNormal, events.EventReasonDeleted, common.Concat("Deleted pod ", pod.Name),
	)
	log.Info("Deleted pod", "name", pod.Name, "namespace", pod.Namespace)

	return reconcile.Result{}, nil
}
