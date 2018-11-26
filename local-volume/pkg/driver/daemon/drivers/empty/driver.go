package empty

import (
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
)

// DriverKind represents the empty driver
const DriverKind = "EMPTY"

// Driver handles empty mounts
type Driver struct {
	MountRes, UnmountRes flex.Response
}
