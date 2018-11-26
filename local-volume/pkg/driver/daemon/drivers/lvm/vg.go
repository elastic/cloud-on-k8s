package lvm

import (
	"fmt"
	"os/exec"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
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
func LookupVolumeGroup(name string) (VolumeGroup, error) {
	result := vgsOutput{}
	cmd := exec.Command("vgs", "--options=vg_name,vg_free", name,
		"--reportformat=json", "--units=b", "--nosuffix")
	if err := cmdutil.RunLVMCmd(cmd, &result); err != nil {
		if isVolumeGroupNotFound(err) {
			return VolumeGroup{}, ErrVolumeGroupNotFound
		}
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
func (vg VolumeGroup) CreateLogicalVolume(name string, sizeInBytes uint64) (LogicalVolume, error) {
	if err := ValidateLogicalVolumeName(name); err != nil {
		return LogicalVolume{}, err
	}
	// size must be a multiple of 512
	roundedSize := roundUpTo512(sizeInBytes)

	cmd := exec.Command("lvcreate", fmt.Sprintf("--size=%db", roundedSize), fmt.Sprintf("--name=%s", name), vg.name)
	if err := cmdutil.RunLVMCmd(cmd, nil); err != nil {
		if isInsufficientSpace(err) {
			return LogicalVolume{}, ErrNoSpace
		}
		if isInsufficientDevices(err) {
			return LogicalVolume{}, ErrTooFewDisks
		}
		return LogicalVolume{}, err
	}

	return LogicalVolume{name, sizeInBytes, vg}, nil
}

// LookupLogicalVolume looks up the logical volume with the given name
// in the current volume group
func (vg VolumeGroup) LookupLogicalVolume(name string) (*LogicalVolume, error) {
	result := lvsOutput{}
	cmd := exec.Command("lvs", "--options=lv_name,lv_size,vg_name", vg.name,
		"--reportformat=json", "--units=b", "--nosuffix")
	if err := cmdutil.RunLVMCmd(cmd, &result); err != nil {
		if isLogicalVolumeNotFound(err) {
			return nil, ErrLogicalVolumeNotFound
		}
		return nil, err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			if lv.VgName != vg.name {
				continue
			}
			if lv.Name != name {
				continue
			}
			return &LogicalVolume{lv.Name, lv.LvSize, vg}, nil
		}
	}
	return nil, ErrLogicalVolumeNotFound
}
