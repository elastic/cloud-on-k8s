package license

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/finalizer"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/watches"
	"k8s.io/apimachinery/pkg/types"
)

// watchName names the license watch for a specific ES cluster.
func watchName(esName types.NamespacedName) string {
	return esName.Name + "-license-watch"
}

// ensureLicenseWatch ensures a watch is registered for a given clusters license
func ensureLicenseWatch(esName types.NamespacedName, w watches.DynamicWatches) error {
	return w.ClusterLicense.AddHandler(watches.NamedWatch{
		Name:    watchName(esName),
		Watcher: esName,
		Watched: esName, // For now simply has the same name will change once pool is introduced
	})
}

// Finalizer ensures any registered license watches are removed on cluster deletion.
func Finalizer(esName types.NamespacedName, w watches.DynamicWatches) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: "license-watch-finalizer",
		Execute: func() error {
			w.ClusterLicense.RemoveHandlerForKey(watchName(esName))
			return nil
		},
	}
}
