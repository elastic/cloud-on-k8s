package lvm

const DriverKind = "LVM"

// Default driver options
const (
	DefaultVolumeGroup    = "elastic-local-vg"
	DefaultUseThinVolumes = false
	DefaultThinPoolName   = "elastic-local-thinpool"
)

// Options defines parameters for the LVM driver
type Options struct {
	VolumeGroupName string
	UseThinVolumes  bool
	ThinPoolName    string
}

// Driver handles LVM mounts
type Driver struct {
	options Options
}

// NewDriver creates a new lvm.Driver with the given options
func NewDriver(options Options) *Driver {
	return &Driver{options: options}
}
