package k8s

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

// UpdatePVNodeAffinity updates the NodeAffinity section of
// the persistent volume with the given name
func (c *Client) UpdatePVNodeAffinity(pvName string, nodeName string) error {
	// retrieve PV with the given name
	pvClient := c.ClientSet.CoreV1().PersistentVolumes()
	pv, err := pvClient.Get(pvName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// set node affinity
	pv.Spec.NodeAffinity = &v1.VolumeNodeAffinity{
		Required: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      apis.LabelHostname,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{nodeName},
						},
					},
				},
			},
		},
	}

	// update resource
	if _, err := pvClient.Update(pv); err != nil {
		return err
	}

	return nil
}
