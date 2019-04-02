// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/provider"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

// BindPVToNode updates the NodeAffinity and labels sections
// of the persistent volume with the given name.
// It returns true if the resource was updated, false otherwise.
func (c *Client) BindPVToNode(pvName string, nodeName string) (bool, error) {
	// retrieve PV with the given name
	pvClient := c.ClientSet.CoreV1().PersistentVolumes()
	pv, err := pvClient.Get(pvName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}

	// update node affinity and labels if needed
	naUpdateRequired := updatePVNodeAffinity(pv, nodeName)
	labelUpdateRequired := updatePVLabelsForNode(pv, nodeName)

	if naUpdateRequired || labelUpdateRequired {
		// update resource
		if _, err := pvClient.Update(pv); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// updatePVNodeAffinity updates the NodeAffinity section of the given PV with the given node name
func updatePVNodeAffinity(pv *v1.PersistentVolume, nodeName string) bool {
	expected := v1.NodeSelectorRequirement{
		Key:      apis.LabelHostname,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{nodeName},
	}

	// check if already up-to-date
	// many things can be nil, so this does not look very beautiful :)
	if na := pv.Spec.NodeAffinity; na != nil {
		if required := na.Required; required != nil {
			for _, t := range required.NodeSelectorTerms {
				for _, r := range t.MatchExpressions {
					if r.Key == expected.Key &&
						r.Operator == expected.Operator &&
						len(r.Values) == 1 && r.Values[0] == expected.Values[0] {
						// value already updated, nothing to do here
						return false
					}
				}
			}
		}
	}

	// update is required: set node affinity
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

	return true
}

// updatePVLabelsForNode updates the node affinity label of the given PV with the given node name
func updatePVLabelsForNode(pv *v1.PersistentVolume, nodeName string) bool {
	current, exists := pv.Labels[provider.NodeAffinityLabel]
	if exists && current == nodeName {
		// already up-to-date
		return false
	}
	labels := pv.Labels
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	labels[provider.NodeAffinityLabel] = nodeName
	pv.Labels = labels
	return true
}

// NewPersistentVolume creates an empty persistent volume with the given name
func NewPersistentVolume(name string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}
