package driver

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/stringsutil"
)

// GarbageCollectPVCs ensures PersistentVolumeClaims created for the given es resource are deleted
// when no longer used, since this is not done automatically by the StatefulSet controller.
// Related issue in the k8s repo: https://github.com/kubernetes/kubernetes/issues/55045
// PVCs that are not supposed to exist given the actual and expected StatefulSets are removed.
// This covers:
// * leftover PVCs created for StatefulSets that do not exist anymore
// * leftover PVCs created for StatefulSets replicas that don't exist anymore (eg. downscale from 5 to 3 nodes)
func GarbageCollectPVCs(
	k8sClient k8s.Client,
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
	for _, pvc := range pvcsToRemove(pvcs.Items, actualStatefulSets, expectedStatefulSets) {
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
	expectedPVCs := stringsutil.SliceToMap(append(actualStatefulSets.PVCNames(), expectedStatefulSets.PVCNames()...))
	var toRemove []corev1.PersistentVolumeClaim
	for _, pvc := range pvcs {
		if _, exists := expectedPVCs[pvc.Name]; exists {
			continue
		}
		toRemove = append(toRemove, pvc)
	}
	return toRemove
}
