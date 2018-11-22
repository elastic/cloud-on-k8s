package bindmount

// DriverKind represents the bind mount driver
const DriverKind = "BINDMOUNT"

// Driver handles bind mounts
type Driver struct{}

// NewDriver creates a new bindmount.Driver
func NewDriver() *Driver {
	return &Driver{}
}
