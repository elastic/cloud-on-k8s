package elasticsearch

import (
	"context"
	"fmt"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourcesState contains information about a deployments resources.
type ResourcesState struct {
	// AllPods are all the Elasticsearch pods related to the Elasticsearch cluster, including ones with a
	// DeletionTimestamp tombstone set.
	AllPods []corev1.Pod
	// CurrentPods are all non-deleted Elasticsearch pods related to the Elasticsearch cluster.
	CurrentPods []corev1.Pod
	// PVCs are all the PVCs related to this deployment.
	PVCs []corev1.PersistentVolumeClaim
}

// NewResourcesStateFromAPI reflects the current ResourcesState from the API
func NewResourcesStateFromAPI(c client.Client, stack deploymentsv1alpha1.Stack) (*ResourcesState, error) {
	labelSelector, err := NewLabelSelectorForStack(stack)
	if err != nil {
		return nil, err
	}

	allPods, err := getPods(c, stack, labelSelector, nil)
	if err != nil {
		return nil, err
	}

	currentPods := make([]corev1.Pod, 0, len(allPods))
	// filter out pods scheduled for deletion
	for _, p := range allPods {
		if p.DeletionTimestamp != nil {
			log.Info(fmt.Sprintf("Ignoring pod %s scheduled for deletion", p.Name))
			continue
		}
		currentPods = append(currentPods, p)
	}

	pvcs, err := getPersistentVolumeClaims(c, stack, labelSelector, nil)

	esState := ResourcesState{
		AllPods:     allPods,
		CurrentPods: currentPods,
		PVCs:        pvcs,
	}

	return &esState, nil
}

// FindPVCByName looks up a PVC by claim name.
func (state ResourcesState) FindPVCByName(name string) (corev1.PersistentVolumeClaim, error) {
	for _, pvc := range state.PVCs {
		if pvc.Name == name {
			return pvc, nil
		}
	}
	return corev1.PersistentVolumeClaim{}, fmt.Errorf("no PVC named %s found", name)
}

// getPods returns list of pods in the current namespace with a specific set of selectors.
func getPods(
	c client.Client,
	stack deploymentsv1alpha1.Stack,
	labelSelectors labels.Selector,
	fieldSelectors fields.Selector,
) ([]corev1.Pod, error) {
	var podList corev1.PodList

	listOpts := client.ListOptions{
		Namespace:     stack.Namespace,
		LabelSelector: labelSelectors,
		FieldSelector: fieldSelectors,
	}

	if err := c.List(context.TODO(), &listOpts, &podList); err != nil {
		return nil, err
	}

	return podList.Items, nil
}

// getPersistentVolumeClaims returns a list of PVCs in the current namespace with a specific set of selectors.
func getPersistentVolumeClaims(
	c client.Client,
	stack deploymentsv1alpha1.Stack,
	labelSelectors labels.Selector,
	fieldSelectors fields.Selector,
) ([]corev1.PersistentVolumeClaim, error) {
	var pvcs corev1.PersistentVolumeClaimList

	listOpts := client.ListOptions{
		Namespace:     stack.Namespace,
		LabelSelector: labelSelectors,
		FieldSelector: fieldSelectors,
	}

	if err := c.List(context.TODO(), &listOpts, &pvcs); err != nil {
		return nil, err
	}

	return pvcs.Items, nil
}
