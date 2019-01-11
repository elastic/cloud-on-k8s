package driver

import (
	"context"
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/events"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/migration"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/pod"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/reconcilehelper"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// createElasticsearchPod creates the given elasticsearch pod
func createElasticsearchPod(
	c client.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	state *reconcilehelper.ReconcileState,
	pod v1.Pod,
	podSpecCtx pod.PodSpecContext,
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

// deleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func deleteElasticsearchPod(
	c client.Client,
	reconcileState *reconcilehelper.ReconcileState,
	resourcesState support.ResourcesState,
	observedState support.ObservedState,
	pod v1.Pod,
	allDeletions []v1.Pod,
	preDelete func() error,
) (reconcile.Result, error) {
	isMigratingData := migration.IsMigratingData(observedState, pod, allDeletions)
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
