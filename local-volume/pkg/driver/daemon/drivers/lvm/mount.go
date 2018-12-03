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

const defaultSize uint64 = 1000000000 // 1GB, applies if storage size is not specified

// Mount mounts a formated LVM logical volume according to the given params
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	// parse requested storage size, or use default
	requestedSize := uint64(params.Options.SizeBytes)
	if requestedSize == 0 {
		requestedSize = defaultSize
	}

	vg, err := LookupVolumeGroup(d.options.FactoryFunc, d.options.VolumeGroupName)
	if err != nil {
		return flex.Failure(fmt.Sprintf("volume group %s does not seem to exist", d.options.VolumeGroupName))
	}

	// build logical volume name based on PVC name
	lvName := pathutil.ExtractPVCID(params.TargetDir)

	// check if lv already exists, and reuse
	// TODO: call LookupLogicalVolume()

	lv, err := d.CreateLV(vg, lvName, requestedSize)
	if err != nil {
		return flex.Failure(err.Error())
	}

	lvPath, err := lv.Path(d.options.FactoryFunc)
	if err != nil {
		return flex.Failure(fmt.Sprintf("cannot retrieve logical volume device path: %s", err.Error()))
	}

	if err := diskutil.FormatDevice(d.options.FactoryFunc, lvPath, fsType); err != nil {
		return flex.Failure(fmt.Sprintf("cannot format logical volume %s as %s: %s", lv.name, fsType, err.Error()))
	}

	// mount device to the pods dir
	if err := diskutil.MountDevice(d.options.FactoryFunc, lvPath, params.TargetDir); err != nil {
		return flex.Failure(fmt.Sprintf("cannot mount device %s to %s: %s", lvPath, params.TargetDir, err.Error()))
	}

	return flex.Success("successfully created the volume")
}

// CreateLV creates a logical volume from the given settings
func (d *Driver) CreateLV(vg VolumeGroup, name string, size uint64) (LogicalVolume, error) {
	if d.options.UseThinVolumes {
		return d.createThinLV(vg, name, size)
	}

	return d.createStandardLV(vg, name, size)
}

// createThinLV creates a thin volume
func (d *Driver) createThinLV(vg VolumeGroup, name string, virtualSize uint64) (LogicalVolume, error) {
	thinPool, err := vg.GetOrCreateThinPool(d.options.FactoryFunc, d.options.ThinPoolName)
	if err != nil {
		return LogicalVolume{}, fmt.Errorf(fmt.Sprintf("cannot get or create thin pool %s: %s", d.options.ThinPoolName, err.Error()))
	}
	lv, err := thinPool.CreateThinVolume(d.options.FactoryFunc, name, virtualSize)
	if err != nil {
		return LogicalVolume{}, fmt.Errorf(fmt.Sprintf("cannot create thin volume: %s", err.Error()))
	}
	return lv, nil
}

// createThinLV creates a standard logical volume
func (d *Driver) createStandardLV(vg VolumeGroup, name string, size uint64) (LogicalVolume, error) {
	if vg.bytesFree < size {
		return LogicalVolume{}, fmt.Errorf("not enough space left on volume group: available %d bytes, requested: %d bytes", vg.bytesFree, size)
	}
	lv, err := vg.CreateLogicalVolume(d.options.FactoryFunc, name, size)
	if err != nil {
		return LogicalVolume{}, fmt.Errorf("cannot create logical volume: %s", err.Error())
	}
	return lv, nil
}
