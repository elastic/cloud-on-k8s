package finalizer

import (
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
)

// RemoveAll removes all existing Finalizers on an Object
func RemoveAll(c k8s.Client, obj runtime.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	if len(accessor.GetFinalizers()) == 0 {
		return nil
	}
	accessor.SetFinalizers([]string{})
	return c.Update(obj)
}
