package empty

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

// Mount mounts a directory to be used as volume by a pod
// The requested storage size is ignored here.
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	return d.MountRes
}
