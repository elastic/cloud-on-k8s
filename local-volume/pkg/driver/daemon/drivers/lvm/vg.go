package lvm

import (
	"fmt"
	"strconv"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// VolumeGroup represents a volumegroup with a name
type VolumeGroup struct {
	name      string
	bytesFree uint64
}

// vgsOutput is the output struct of the vgs command
type vgsOutput struct {
	Report []struct {
		Vg []struct {
			Name              string `json:"vg_name"`
			UUID              string `json:"vg_uuid"`
			VgSize            uint64 `json:"vg_size,string"`
			VgFree            uint64 `json:"vg_free,string"`
			VgExtentSize      uint64 `json:"vg_extent_size,string"`
			VgExtentCount     uint64 `json:"vg_extent_count,string"`
			VgFreeExtentCount uint64 `json:"vg_free_count,string"`
			VgTags            string `json:"vg_tags"`
		} `json:"vg"`
	} `json:"report"`
}

// LookupVolumeGroup returns the volume group with the given name
func LookupVolumeGroup(newCmd cmdutil.ExecutableFactory, name string) (VolumeGroup, error) {
	var result vgsOutput
	cmd := newCmd(
		"vgs",
		"--options=vg_name,vg_free",
		"--reportformat=json", "--units=b", "--nosuffix",
		name,
	)
	if err := RunLVMCmd(cmd, &result); err != nil {
		return VolumeGroup{}, err
	}
	for _, report := range result.Report {
		for _, vg := range report.Vg {
			return VolumeGroup{
				name:      vg.Name,
				bytesFree: vg.VgFree,
			}, nil
		}
	}
	return VolumeGroup{}, ErrVolumeGroupNotFound
}

// roundUpTo512 rounds the given number up to a 512 multiple
func roundUpTo512(n uint64) uint64 {
	return ((n + 512) / 512) * 512
}

// CreateLogicalVolume creates a logical volume of the given device
// and size.
//
// The actual size may be larger than asked for as the smallest
// increment is the size of an extent on the volume group in question.
func (vg VolumeGroup) CreateLogicalVolume(newCmd cmdutil.ExecutableFactory, name string, sizeInBytes uint64) (LogicalVolume, error) {
	if err := ValidateLogicalVolumeName(name); err != nil {
		return LogicalVolume{}, err
	}

	// size must be a multiple of 512
	roundedSize := roundUpTo512(sizeInBytes)

	cmd := newCmd(
		"lvcreate",
		fmt.Sprintf("--size=%db", roundedSize),
		"--name", name,
		vg.name,
	)

	if err := RunLVMCmd(cmd, nil); err != nil {
		return LogicalVolume{}, err
	}

	return LogicalVolume{name, sizeInBytes, vg}, nil
}

// CreateThinPool creates a thin pool logical volume with the given name and size.
func (vg VolumeGroup) CreateThinPool(newCmd cmdutil.ExecutableFactory, name string) (ThinPool, error) {
	if err := ValidateLogicalVolumeName(name); err != nil {
		return ThinPool{}, err
	}

	cmd := newCmd(
		"lvcreate",
		"--extents", "100%FREE", // use all available space in the vg
		"--thinpool", name,
		vg.name,
	)

	if err := RunLVMCmd(cmd, nil); err != nil {
		return ThinPool{}, err
	}

	return vg.LookupThinPool(newCmd, name)
}

// ListLogicalVolumes the logical volumes
func (vg VolumeGroup) ListLogicalVolumes(newCmd cmdutil.ExecutableFactory) ([]LogicalVolume, error) {
	var result lvsOutput
	cmd := newCmd(
		"lvs",
		"--options=lv_name,lv_size,vg_name,lv_layout,data_percent",
		vg.name,
		"--reportformat=json",
		"--nosuffix",
	)

	if err := RunLVMCmd(cmd, &result); err != nil {
		return nil, err
	}

	var lvs []LogicalVolume
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			lvs = append(lvs, LogicalVolume{lv.Name, lv.LvSize, vg})
		}
	}
	return lvs, nil
}

func (vg VolumeGroup) lookupLV(newCmd cmdutil.ExecutableFactory, name string) (lvsOutput, error) {
	var result lvsOutput
	cmd := newCmd(
		"lvs",
		"--options=lv_name,lv_size,vg_name,lv_layout,data_percent",
		"--reportformat=json", "--units=b", "--nosuffix",
		vg.name,
	)

	err := RunLVMCmd(cmd, &result)
	return result, err
}

// LookupLogicalVolume looks up the logical volume with the given name
// in the current volume group
func (vg VolumeGroup) LookupLogicalVolume(newCmd cmdutil.ExecutableFactory, name string) (LogicalVolume, error) {
	result, err := vg.lookupLV(newCmd, name)
	if err != nil {
		return LogicalVolume{}, err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			if lv.VgName != vg.name {
				continue
			}
			if lv.Name != name {
				continue
			}
			return LogicalVolume{lv.Name, lv.LvSize, vg}, nil
		}
	}
	return LogicalVolume{}, ErrLogicalVolumeNotFound
}

// LookupThinPool returns the thinpool with the given name
func (vg VolumeGroup) LookupThinPool(newCmd cmdutil.ExecutableFactory, name string) (ThinPool, error) {
	result, err := vg.lookupLV(newCmd, name)
	if err != nil {
		return ThinPool{}, err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			var namesNotMatch = lv.VgName != vg.name || lv.Name != name
			var notThinLayout = lv.LvLayout != ThinPoolLayout
			if namesNotMatch || notThinLayout {
				continue
			}
			// parse data percent, which may look like "" or "12.20"
			dataPercent := 0.0
			if lv.DataPercent != "" {
				dataPercent, err = strconv.ParseFloat(lv.DataPercent, 64)
				if err != nil {
					return ThinPool{}, err
				}
			}
			return ThinPool{
				LogicalVolume: LogicalVolume{
					name:        lv.Name,
					sizeInBytes: lv.LvSize,
					vg:          vg,
				},
				dataPercent: dataPercent,
			}, nil
		}
	}
	return ThinPool{}, ErrLogicalVolumeNotFound
}

// GetOrCreateThinPool gets the thinpool with the given name,
// or creates it if it does not exit
func (vg VolumeGroup) GetOrCreateThinPool(newCmd cmdutil.ExecutableFactory, name string) (ThinPool, error) {
	thinPool, err := vg.LookupThinPool(newCmd, name)
	if err != nil {
		if err == ErrLogicalVolumeNotFound {
			return vg.CreateThinPool(newCmd, name)
		}
		return ThinPool{}, err
	}
	return thinPool, nil
}
