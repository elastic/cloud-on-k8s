package daemon

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/bindmount"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/lvm"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/model"
)

// DriverKind represents a driver implementation name
type DriverKind string

// Driver interface to be implemented by drivers
type Driver interface {
	Init() model.Response
	Mount(params model.MountRequest) model.Response
	Unmount(params model.UnmountRequest) model.Response
}

// NewDriver corresponding to the given driver kind
func NewDriver(driverKind string) (Driver, error) {
	switch driverKind {
	case bindmount.DriverKind:
		return bindmount.NewDriver(), nil
	case lvm.DriverKind:
		return lvm.NewDriver(), nil
	default:
		return nil, fmt.Errorf("Invalid driver kind: %s", driverKind)
	}
}
