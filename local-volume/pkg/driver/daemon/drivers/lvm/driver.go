package lvm

const DriverKind = "LVM"
const DefaultVolumeGroup = "elastic-local-vg"

type Driver struct {
	volumeGroupName string
}

func NewDriver(volumeGroupName string) *Driver {
	return &Driver{
		volumeGroupName: volumeGroupName,
	}
}
