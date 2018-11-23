package lvm

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/pathutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
)

// TODO: parametrize
const fsType = "ext4"

const defaultSize uint64 = 1000000 // 1MB, applies if storage size is not specified

// Mount mounts a formated LVM logical volume according to the given params
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	// parse requested storage size, or use default
	requestedSize := uint64(params.Options.SizeBytes)
	if requestedSize == 0 {
		requestedSize = defaultSize
	}

	vg, err := LookupVolumeGroup(d.volumeGroupName)
	if err != nil {
		return flex.Failure(fmt.Sprintf("volume group %s does not seem to exist", d.volumeGroupName))
	}

	if vg.bytesFree < requestedSize {
		return flex.Failure(fmt.Sprintf("Not enough space left on volume group. Available: %d bytes. Requested: %d bytes.", vg.bytesFree, requestedSize))
	}

	// build logical volume name based on PVC name
	lvName := pathutil.ExtractPVCID(params.TargetDir)

	// check if lv already exists, and reuse
	// TODO: call LookupLogicalVolume()

	lv, err := vg.CreateLogicalVolume(lvName, requestedSize)
	if err != nil {
		return flex.Failure(fmt.Sprintf("cannot create logical volume: %s", err.Error()))
	}

	lvPath, err := lv.Path()
	if err != nil {
		return flex.Failure(fmt.Sprintf("cannot retrieve logical volume device path: %s", err.Error()))
	}

	if err := diskutil.FormatDevice(lvPath, fsType); err != nil {
		return flex.Failure(fmt.Sprintf("cannot format logical volume %s as %s: %s", lv.name, fsType, err.Error()))
	}

	// mount device to the pods dir
	if err := diskutil.MountDevice(lvPath, params.TargetDir); err != nil {
		return flex.Failure(fmt.Sprintf("cannot mount device %s to %s: %s", lvPath, params.TargetDir, err.Error()))
	}

	return flex.Success("successfully created the volume")
}
