package bindmount

// DriverKind represents the bind mount driver
const DriverKind = "BINDMOUNT"

// ContainerMountPath is the path to access volumes from within the container
const ContainerMountPath = "/mnt/elastic-local-volumes"

// Driver handles bind mounts
type Driver struct{}

// Info returns some information about the driver
func (d *Driver) Info() string {
	return "Bind mount driver"
}

// NewDriver creates a new bindmount.Driver
func NewDriver() *Driver {
	return &Driver{}
}
