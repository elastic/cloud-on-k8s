package bindmount

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
)

const (
	// DriverKind represents the bind mount driver
	DriverKind = "BINDMOUNT"

	// DefaultContainerMountPath is the path to access volumes from within the container
	DefaultContainerMountPath = "/mnt/elastic-local-volumes"
)

// Driver handles bind mounts
type Driver struct {
	factoryFunc cmdutil.ExecutableFactory
	mountPath   string
}

// Options for the BindMount driver.
type Options struct {
	Factory   cmdutil.ExecutableFactory
	MountPath string
}

// Info returns some information about the driver
func (d *Driver) Info() string {
	return "Bind mount driver"
}

// NewDriver creates a new bindmount.Driver
func NewDriver(opts Options) *Driver {
	return &Driver{
		factoryFunc: opts.Factory,
		mountPath:   opts.MountPath,
	}
}
