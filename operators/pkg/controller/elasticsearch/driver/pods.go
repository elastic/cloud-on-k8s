// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/events"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/migration"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/observer"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
	pvcutils "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pvc"
	esreconcile "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// createElasticsearchPod creates the given elasticsearch pod
func createElasticsearchPod(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
	reconcileState *esreconcile.State,
	pod corev1.Pod,
	podSpecCtx pod.PodSpecContext,
) error {
	orphanedPVCs, err := pvcutils.FindOrphanedVolumeClaims(c, es)
	if err != nil {
		return err
	}

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
		// TODO : we are creating PVC way too far in the process, it's almost too late to compare them with existing ones
		pvc, err := getOrCreatePVC(&pod, claimTemplate, orphanedPVCs, c, scheme, es)
		if err != nil {
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

	// create the node certificates secret for this pod, which is our promise that we will sign a CSR
	// originating from the pod after it has started and produced a CSR
	log.Info("Ensuring that node certificate secret exists for pod", "pod", pod.Name)
	nodeCertificatesSecret, err := nodecerts.EnsureNodeCertificateSecretExists(
		c,
		scheme,
		&es,
		pod,
		nodecerts.LabelNodeCertificateTypeElasticsearchAll,
		// add the cluster name label so we select all the node certificates secrets associated with a cluster easily
		map[string]string{label.ClusterNameLabelName: es.Name},
	)
	if err != nil {
		return err
	}

	// we finally have the node certificates secret made, so we can inject the secret volume into the pod
	nodeCertificatesSecretVolume := volume.NewSecretVolumeWithMountPath(
		nodeCertificatesSecret.Name,
		volume.NodeCertificatesSecretVolumeName,
		volume.NodeCertificatesSecretVolumeMountPath,
	)
	// add the node certificates volume to volumes
	pod.Spec.Volumes = append(pod.Spec.Volumes, nodeCertificatesSecretVolume.Volume())

	if err := controllerutil.SetControllerReference(&es, &pod, scheme); err != nil {
		return err
	}
	if err := c.Create(&pod); err != nil {
		return err
	}
	reconcileState.AddEvent(corev1.EventTypeNormal, events.EventReasonCreated, stringsutil.Concat("Created pod ", pod.Name))
	log.Info("Created pod", "name", pod.Name, "namespace", pod.Namespace)

	return nil
}

// getOrCreatePVC tries to attach a PVC that already exists or attaches a new one otherwise.
func getOrCreatePVC(pod *corev1.Pod,
	claimTemplate corev1.PersistentVolumeClaim,
	orphanedPVCs *pvcutils.OrphanedPersistentVolumeClaims,
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.ElasticsearchCluster,
) (*corev1.PersistentVolumeClaim, error) {
	// Generate the desired PVC from the template
	pvc := newPVCFromTemplate(claimTemplate, pod)
	// Seek for an orphaned PVC that matches the desired one
	orphanedPVC := orphanedPVCs.GetOrphanedVolumeClaim(pod.Labels, pvc)

	if orphanedPVC != nil {
		// ReUSE the orphaned PVC
		pvc = orphanedPVC
		// Update the name of the pod to reflect the change
		podName, err := pvcutils.GetPodNameFromLabels(pvc)
		if err != nil {
			return nil, err
		}
		pod.Name = podName
		log.Info("Reusing PVC", "pod", pod.Name, "pvc", pvc.Name)
		return pvc, nil
	}

	// No match, create a new PVC
	log.Info("Creating PVC", "pod", pod.Name, "pvc", pvc.Name)
	if err := controllerutil.SetControllerReference(&es, pvc, scheme); err != nil {
		return nil, err
	}
	err := c.Create(pvc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, err
	}
	return pvc, nil
}

func newPVCFromTemplate(claimTemplate corev1.PersistentVolumeClaim, pod *corev1.Pod) *corev1.PersistentVolumeClaim {
	pvc := claimTemplate.DeepCopy()
	// generate unique name for this pvc.
	// TODO: this may become too long?
	pvc.Name = pod.Name + "-" + claimTemplate.Name
	pvc.Namespace = pod.Namespace
	// we re-use the labels and annotation from the associated pod, which is used to select these PVCs when
	// reflecting state from K8s.
	pvc.Labels = pod.Labels
	// Add the current pod name as a label
	pvc.Labels[label.NodeNameLabelName] = pod.Name
	pvc.Annotations = pod.Annotations
	// TODO: add more labels or annotations?
	return pvc
}

// deleteElasticsearchPod deletes the given elasticsearch pod,
// unless a data migration is in progress
func deleteElasticsearchPod(
	c k8s.Client,
	reconcileState *esreconcile.State,
	resourcesState esreconcile.ResourcesState,
	observedState observer.State,
	pod corev1.Pod,
	allDeletions []corev1.Pod,
	preDelete func() error,
) (reconcile.Result, error) {
	isMigratingData := migration.IsMigratingData(observedState, pod, allDeletions)
	if isMigratingData {
		log.Info(stringsutil.Concat("Migrating data, skipping deletes because of ", pod.Name))
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

		if err := c.Delete(&pvc); err != nil && !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	}

	if err := preDelete(); err != nil {
		return reconcile.Result{}, err
	}
	if err := c.Delete(&pod); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	reconcileState.AddEvent(
		corev1.EventTypeNormal, events.EventReasonDeleted, stringsutil.Concat("Deleted pod ", pod.Name),
	)
	log.Info("Deleted pod", "name", pod.Name, "namespace", pod.Namespace)

	return reconcile.Result{}, nil
}
