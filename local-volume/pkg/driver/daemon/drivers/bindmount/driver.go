package bindmount

// DriverKind represents the bind mount driver
const DriverKind = "BINDMOUNT"

// ContainerMountPath is the path to access volumes from within the container
const ContainerMountPath = "/mnt/elastic-local-volumes"

// Driver handles bind mounts
type Driver struct{}

// NewDriver creates a new bindmount.Driver
func NewDriver() *Driver {
	return &Driver{}
}
