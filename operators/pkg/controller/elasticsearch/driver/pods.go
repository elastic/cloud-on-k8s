// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package driver

import (
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/certificates/transport"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/name"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	pvcpkg "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pvc"
	pvcutils "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pvc"
	esreconcile "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/reconcile"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/stringsutil"
)

// createElasticsearchPod creates the given elasticsearch pod
func createElasticsearchPod(
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	reconcileState *esreconcile.State,
	pod corev1.Pod,
	podSpecCtx pod.PodSpecContext,
	orphanedPVCs *pvcutils.OrphanedPersistentVolumeClaims,
) error {
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

	for _, claimTemplate := range podSpecCtx.NodeSpec.VolumeClaimTemplates {
		// TODO : we are creating PVC way too far in the process, it's almost too late to compare them with existing ones
		pvc, err := getOrCreatePVC(&pod, claimTemplate, orphanedPVCs, c, scheme, es)
		if err != nil {
			return err
		}

		vol := corev1.Volume{
			Name: claimTemplate.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvc.Name,
					// TODO: support read only pvcs
				},
			},
		}
		pod = replaceVolume(pod, vol)
	}

	// create the transport certificates secret for this pod because it must exist before we're able to create the
	// pod
	log.Info("Ensuring that transport certificate secret exists for pod", "pod", pod.Name)
	transportCertificatesSecret, err := transport.EnsureTransportCertificateSecretExists(
		c,
		scheme,
		es,
		pod,
	)
	if err != nil {
		return err
	}

	// we finally have the transport certificates secret made, so we can inject the secret volume into the pod
	transportCertsVolume := volume.NewSecretVolumeWithMountPath(
		transportCertificatesSecret.Name,
		esvolume.TransportCertificatesSecretVolumeName,
		esvolume.TransportCertificatesSecretVolumeMountPath).Volume()
	pod = replaceVolume(pod, transportCertsVolume)

	// create the config volume for this pod, now that we have a proper name for the pod
	if err := settings.ReconcileConfig(c, es, pod, podSpecCtx.Config); err != nil {
		return err
	}
	configSecretVolume := settings.ConfigSecretVolume(pod.Name).Volume()
	pod = replaceVolume(pod, configSecretVolume)

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

// replaceVolume replaces an existing volume in the pod that has the same name as the given one.
func replaceVolume(pod corev1.Pod, volume corev1.Volume) corev1.Pod {
	for i, v := range pod.Spec.Volumes {
		if v.Name == volume.Name {
			pod.Spec.Volumes[i] = volume
			break
		}
	}
	return pod
}

// getOrCreatePVC tries to attach a PVC that already exists or attaches a new one otherwise.
func getOrCreatePVC(pod *corev1.Pod,
	claimTemplate corev1.PersistentVolumeClaim,
	orphanedPVCs *pvcutils.OrphanedPersistentVolumeClaims,
	c k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
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
	pvc.Name = name.NewPVCName(pod.Name, claimTemplate.Name)
	pvc.Namespace = pod.Namespace
	// reuse some labels also applied to the pod for comparison purposes
	if pvc.Labels == nil {
		pvc.Labels = map[string]string{}
	}
	for _, k := range pvcpkg.PodLabelsInPVCs {
		pvc.Labels[k] = pod.Labels[k]
	}
	// Add the current pod name as a label
	pvc.Labels[label.PodNameLabelName] = pod.Name
	pvc.Annotations = pod.Annotations
	// TODO: add more labels or annotations?
	return pvc
}

// deleteElasticsearchPod deletes the given elasticsearch pod. Tests to check if the pod can be safely deleted must
// be done before the call to this function.
func deleteElasticsearchPod(
	c k8s.Client,
	reconcileState *esreconcile.State,
	resourcesState esreconcile.ResourcesState,
	pod corev1.Pod,
	preDelete func() error,
) (reconcile.Result, error) {

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

	// delete configuration for that pod (would be garbage collected otherwise)
	secret, err := settings.GetESConfigSecret(c, k8s.ExtractNamespacedName(&pod))
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	if err = c.Delete(&secret); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
