package lvm

const DriverKind = "LVM"
const DefaultVolumeGroup = "elastic-local-vg"

// Options defines parameters for the LVM driver
type Options struct {
	VolumeGroupName string
}

// Driver handles LVM mounts
type Driver struct {
	volumeGroupName string
}

// NewDriver creates a new lvm.Driver with the given options
func NewDriver(opts Options) *Driver {
	return &Driver{volumeGroupName: opts.VolumeGroupName}
}
