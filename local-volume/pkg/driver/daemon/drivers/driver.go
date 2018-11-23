package drivers

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers/bindmount"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/drivers/lvm"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
)

// DriverKind represents a driver implementation name
type DriverKind string

// Driver interface to be implemented by drivers
type Driver interface {
	Init() flex.Response
	Mount(params protocol.MountRequest) flex.Response
	Unmount(params protocol.UnmountRequest) flex.Response
}

// NewDriver corresponding to the given driver kind
func NewDriver(driverKind string, lvmVolumeGroup string) (Driver, error) {
	switch driverKind {
	case bindmount.DriverKind:
		return bindmount.NewDriver(), nil
	case lvm.DriverKind:
		return lvm.NewDriver(lvmVolumeGroup), nil
	default:
		return nil, fmt.Errorf("Invalid driver kind: %s", driverKind)
	}
}
