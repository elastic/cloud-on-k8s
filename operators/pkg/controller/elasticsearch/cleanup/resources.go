package cleanup

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("cleanup")

// DeleteAfter represents how long after creation an object can be safely garbage collected.
const DeleteAfter = 10 * time.Minute

// IsTooYoungForGC checks the object creation time, and returns true
// if we consider it should not be garbage collected yet.
// This is to avoid situations where we would delete an object that was just created,
// or delete an object due to a (temporary) out-of-sync cache.
func IsTooYoungForGC(object metav1.Object) bool {
	creationTime := object.GetCreationTimestamp()
	if time.Now().Sub(creationTime.Time) > DeleteAfter {
		return false
	}
	return true
}

// DeleteOrphanedResources cleans up resources that are not needed anymore for the given es cluster.
func DeleteOrphanedResources(c k8s.Client, es v1alpha1.Elasticsearch) error {
	var resources []runtime.Object
	var secrets corev1.SecretList
	if err := c.List(&client.ListOptions{
		Namespace:     es.Namespace,
		LabelSelector: label.NewLabelSelectorForElasticsearch(es),
	}, &secrets); err != nil {
		return err
	}
	for i := range secrets.Items {
		resources = append(resources, &secrets.Items[i])
	}
	return cleanupFromPodReference(c, es.Namespace, resources)
}

// cleanupFromPodReference deletes objects having a reference to
// a pod which does not exist anymore.
func cleanupFromPodReference(c k8s.Client, namespace string, objects []runtime.Object) error {
	for _, runtimeObj := range objects {
		obj, err := meta.Accessor(runtimeObj)
		if err != nil {
			return err
		}
		podName, hasPodReference := obj.GetLabels()[label.PodNameLabelName]
		if !hasPodReference {
			continue
		}
		// this secret applies to a particular pod
		// remove it if the pod does not exist anymore
		var pod corev1.Pod
		err = c.Get(types.NamespacedName{
			Namespace: namespace,
			Name:      podName,
		}, &pod)
		if apierrors.IsNotFound(err) {
			if IsTooYoungForGC(obj) {
				// pod might not be there in the cache yet,
				// skip deletion
				continue
			}
			// pod does not exist anymore, delete the object
			log.Info("Garbage-collecting resource", "namespace", namespace, "name", obj.GetName())
			if deleteErr := c.Delete(runtimeObj); deleteErr != nil {
				return deleteErr
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
