// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/pathutil"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/flex"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/protocol"
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

	vg, err := LookupVolumeGroup(d.options.ExecutableFactory, d.options.VolumeGroupName)
	if err != nil {
		return flex.Failure(fmt.Sprintf("volume group %s does not seem to exist", d.options.VolumeGroupName))
	}

	// build logical volume name based on PVC name
	lvName := pathutil.ExtractPVCID(params.TargetDir)

	// TODO: only the volume name is used, not the specifications of the logical volumes
	existingLV, err := vg.LookupLogicalVolume(d.options.ExecutableFactory, lvName)
	if err != nil && err != ErrLogicalVolumeNotFound {
		return flex.Failure(fmt.Sprintf("error while looking for logical volume: %s", err.Error()))
	}

	var logicalVolume LogicalVolume
	var lvPath string

	if err == ErrLogicalVolumeNotFound {
		// Create a new logical volume
		logicalVolume, err = d.CreateLV(vg, lvName, requestedSize)
		if err != nil {
			return flex.Failure(err.Error())
		}

		lvPath, err = logicalVolume.Path(d.options.ExecutableFactory)
		if err != nil {
			return flex.Failure(fmt.Sprintf("cannot retrieve logical volume device path: %s", err.Error()))
		}

		if err := diskutil.FormatDevice(d.options.ExecutableFactory, lvPath, fsType); err != nil {
			return flex.Failure(fmt.Sprintf("cannot format logical volume %s as %s: %s", logicalVolume.name, fsType, err.Error()))
		}
	} else {
		// ReUse the logical volume
		logicalVolume = existingLV
		lvPath, err = logicalVolume.Path(d.options.ExecutableFactory)
		if err != nil {
			return flex.Failure(fmt.Sprintf("cannot retrieve logical volume device path: %s", err.Error()))
		}
	}

	// mount device to the pods dir
	if err := diskutil.MountDevice(d.options.ExecutableFactory, lvPath, params.TargetDir); err != nil {
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
	thinPool, err := vg.GetOrCreateThinPool(d.options.ExecutableFactory, d.options.ThinPoolName)
	if err != nil {
		return LogicalVolume{}, fmt.Errorf(fmt.Sprintf("cannot get or create thin pool %s: %s", d.options.ThinPoolName, err.Error()))
	}
	lv, err := thinPool.CreateThinVolume(d.options.ExecutableFactory, name, virtualSize)
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
	lv, err := vg.CreateLogicalVolume(d.options.ExecutableFactory, name, size)
	if err != nil {
		return LogicalVolume{}, fmt.Errorf("cannot create logical volume: %s", err.Error())
	}
	return lv, nil
}
