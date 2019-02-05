package empty

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
)

// DriverKind represents the empty driver
const DriverKind = "EMPTY"

// Driver handles empty mounts
type Driver struct {
	MountRes, UnmountRes flex.Response
}

func (d *Driver) Info() string {
	return "Empty driver"
}

func (d *Driver) ListVolumes() ([]string, error) {
	panic("implement me")
}

func (d *Driver) PurgeVolume(volumeName string) error {
	panic("implement me")
}

var _ drivers.Driver = &Driver{}
