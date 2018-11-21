package stack

import (
	"context"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/pkg/controller/stack/common"
	"github.com/elastic/stack-operators/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileNodes lists all of the kubernetes nodes in the cluster checking for
// any nodes that are marked as "Unschedulable". When that is the case any of
// the Stack elasticsearch pods are annotated with at "tainted" tag. It is up to
// other parts of the codebase to make that tag actionable or not.
func (r *ReconcileStack) reconcileNodes() (reconcile.Result, error) {
	var nodes corev1.NodeList
	if err := r.List(context.TODO(), &client.ListOptions{}, &nodes); err != nil {
		return reconcile.Result{}, err
	}

	var nodeNames []string
	for _, n := range nodes.Items {
		if n.Spec.Unschedulable {
			nodeNames = append(nodeNames, n.Name)
		}
	}

	allPods, err := elasticsearch.GetPods(r.Client, deploymentsv1alpha1.Stack{}, elasticsearch.TypeSelector, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	var podsToUpdate []corev1.Pod
	for _, pod := range allPods {
		if common.StringInSlice(pod.Spec.NodeName, nodeNames) {
			if pod.Annotations == nil {
				pod.Annotations = make(map[string]string, 1)
			}

			if _, ok := pod.Annotations[elasticsearch.TaintedAnnotationName]; ok {
				continue
			}

			pod.Annotations[elasticsearch.TaintedAnnotationName] = "true"
			podsToUpdate = append(podsToUpdate, pod)
			log.Info("Tagging pod for eviction from cordoned node", "pod", pod.Name)
		}
	}

	for _, pod := range podsToUpdate {
		if err := r.Client.Update(context.TODO(), &pod); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}
