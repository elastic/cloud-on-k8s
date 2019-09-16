package driver

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// GarbageCollectPVCs ensures PersistentVolumeClaims created for the given es resource are deleted
// when no longer used, since this is not done automatically by the StatefulSet controller.
// Related issue in the k8s repo: https://github.com/kubernetes/kubernetes/issues/55045
// It sets an owner reference to automatically delete PVCs on es deletion, and garbage collects
// unused PVCs of existing StatefulSets.
// Note we do **not** delete the corresponding PersistentVolumes but just the PersistentVolumeClaims.
// PV deletion is left to the responsibility of the storage class reclaim policy.
func GarbageCollectPVCs(
	k8sClient k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	actualStatefulSets sset.StatefulSetList,
	expectedStatefulSets sset.StatefulSetList,
) error {
	// PVCs are using the same labels as their corresponding StatefulSet, so we can filter on ES cluster name.
	var pvcs corev1.PersistentVolumeClaimList
	if err := k8sClient.List(&client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
	}, &pvcs); err != nil {
		return err
	}
	if err := reconcilePVCsOwnerRef(k8sClient, scheme, es, pvcs.Items); err != nil {
		return err
	}
	return deleteUnusedPVCs(k8sClient, pvcs.Items, actualStatefulSets, expectedStatefulSets)
}

// reconcilePVCsOwnerRef ensures PVCs created for this Elasticsearch cluster have an owner ref set to
// the Elasticsearch resource, so they are deleted automatically upon Elasticsearch deletion.
// A subtle race condition exists: users may still end up with leftover PVCs if the Elasticsearch resource
// gets deleted right after creation or update, before this is called.
func reconcilePVCsOwnerRef(
	k8sClient k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	pvcs []corev1.PersistentVolumeClaim,
) error {
	for _, pvc := range pvcs {
		if err := setPVCOwnerRef(k8sClient, scheme, es, pvc); err != nil {
			return err
		}
	}
	return nil
}

// setPVCOwnerRef sets an owner reference targeting es on the given pvc, if not already set.
func setPVCOwnerRef(
	k8sClient k8s.Client,
	scheme *runtime.Scheme,
	es v1alpha1.Elasticsearch,
	pvc corev1.PersistentVolumeClaim,
) error {
	for _, ref := range pvc.OwnerReferences {
		if ref.Name == es.Name {
			// already set, nothing to do
			return nil
		}
	}
	log.V(1).Info("Setting PersistentVolumeClaim owner reference",
		"namespace", es.Namespace,
		"es_name", es.Name,
		"pvc_name", pvc.Name,
	)
	if err := controllerutil.SetControllerReference(&es, &pvc, scheme); err != nil {
		return err
	}
	return k8sClient.Update(&pvc)
}

// deleteUnusedPVCs deletes PVC resources that are not required anymore for this cluster.
func deleteUnusedPVCs(
	k8sClient k8s.Client,
	pvcs []corev1.PersistentVolumeClaim,
	actualStatefulSets sset.StatefulSetList,
	expectedStatefulSets sset.StatefulSetList,
) error {
	for _, pvc := range pvcsToRemove(pvcs, actualStatefulSets, expectedStatefulSets) {
		log.Info("Deleting PVC", "namespace", pvc.Namespace, "pvc_name", pvc.Name)
		if err := k8sClient.Delete(&pvc); err != nil {
			return err
		}
	}
	return nil
}

// pvcsToRemove filters the given pvcs to ones that can be safely removed based on Pods
// of actual and expected StatefulSets.
func pvcsToRemove(
	pvcs []corev1.PersistentVolumeClaim,
	actualStatefulSets sset.StatefulSetList,
	expectedStatefulSets sset.StatefulSetList,
) []corev1.PersistentVolumeClaim {
	// Build the list of PVCs from both actual & expected StatefulSets (may contain duplicate entries).
	// The list may contain PVCs for Pods that do not exist (eg. not created yet), but does not
	// consider Pods in the process of being deleted (but not deleted yet), since already covered
	// by checking expectations earlier in the process.
	// Then, just return existing PVCs that are not part of that list.
	expectedPVCs := append(actualStatefulSets.PVCNames(), expectedStatefulSets.PVCNames()...)
	var toRemove []corev1.PersistentVolumeClaim
	for _, pvc := range pvcs {
		if !stringsutil.StringInSlice(pvc.Name, expectedPVCs) {
			toRemove = append(toRemove, pvc)
		}
	}
	return toRemove
}
