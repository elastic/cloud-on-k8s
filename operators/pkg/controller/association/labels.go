package association

import (
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// AssociationLabelName marks resources created by this controller for easier retrieval.
	AssociationLabelName = "associations.k8s.elastic.co/name"
)

// NewResourceSelector selects resources labeled as related to the named association.
func NewResourceSelector(name string) labels.Selector {
	return labels.Set(map[string]string{AssociationLabelName: name}).AsSelector()
}
