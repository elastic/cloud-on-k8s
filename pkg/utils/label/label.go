package labels

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// HasLabel takes in a runtime.Object and a set of label keys as parameters.
// It returns true if the object contains all the labels and false otherwise.
func HasLabel(obj runtime.Object, labels ...string) bool {
	// if input labels provided are empty, fail fast
	if len(labels) == 0 {
		return false
	}
	for _, label := range labels {
		o, err := meta.Accessor(obj)
		if err != nil {
			return false
		}
		ol := o.GetLabels()
		if _, exists := ol[label]; !exists {
			return false
		}
	}
	return true
}
