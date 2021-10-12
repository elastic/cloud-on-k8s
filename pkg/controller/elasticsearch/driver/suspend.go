package driver

import (
	"context"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/sset"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func reconcileSuspendedPods(c k8s.Client, es esv1.Elasticsearch) error {
	suspended, exists := es.Annotations[initcontainer.SuspendAnnotation]
	if !exists {
		return nil
	}

	podNames := strings.Split(suspended, ",")
	suspendedPods := make(map[string]struct{})
	for _, p := range podNames {
		suspendedPods[strings.TrimSpace(p)] = struct{}{}
	}
	statefulSets, err := sset.RetrieveActualStatefulSets(c, k8s.ExtractNamespacedName(&es))
	if err != nil {
		return err
	}
	knownPodNames := statefulSets.PodNames()

	for _, podName := range knownPodNames {
		if _, suspended := suspendedPods[podName]; suspended {
			var pod corev1.Pod
			if err := c.Get(context.Background(), types.NamespacedName{Namespace: es.Namespace, Name: podName}, &pod); err != nil {
				return err
			}
			for _, s := range pod.Status.ContainerStatuses {
				// delete the Pod without grace period if the main container is running
				if s.Name == esv1.ElasticsearchContainerName && s.State.Running != nil {
					log.Info("Deleting suspended pod", "pod_name", pod.Name, "pod_uid", pod.UID,
						"namespace", es.Namespace, "es_name", es.Name)
					if err := c.Delete(context.Background(), &pod, client.GracePeriodSeconds(0)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
