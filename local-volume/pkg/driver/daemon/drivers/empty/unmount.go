package empty

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

// Unmount unmounts the persistent volume
func (d *Driver) Unmount(params protocol.UnmountRequest) flex.Response {
	return d.UnmountRes
}
