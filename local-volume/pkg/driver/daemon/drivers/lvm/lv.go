// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"regexp"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// LogicalVolume represents an LVM logical volume
type LogicalVolume struct {
	name        string
	sizeInBytes uint64
	vg          VolumeGroup
}

// lvsOutput is the output struct of the lvs command
type lvsOutput struct {
	Report []struct {
		Lv []struct {
			Name        string `json:"lv_name"`
			VgName      string `json:"vg_name"`
			LvPath      string `json:"lv_path"`
			LvSize      uint64 `json:"lv_size,string"`
			LvTags      string `json:"lv_tags"`
			LvLayout    string `json:"lv_layout"`
			DataPercent string `json:"data_percent"` // uint64 as string, that can be empty
		} `json:"lv"`
	} `json:"report"`
}

// Path returns the device path for the logical volume.
func (lv LogicalVolume) Path(newCmd cmdutil.ExecutableFactory) (string, error) {
	result := lvsOutput{}
	cmd := newCmd(
		"lvs",
		"--options=lv_path",
		"--reportformat=json", "--units=b", "--nosuffix",
		lv.vg.name+"/"+lv.name,
	)
	if err := RunLVMCmd(cmd, &result); err != nil {
		return "", err
	}
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			return lv.LvPath, nil
		}
	}
	return "", ErrLogicalVolumeNotFound
}

// Remove the logical volume from the volume group
func (lv LogicalVolume) Remove(newCmd cmdutil.ExecutableFactory) error {
	return RunLVMCmd(newCmd(
		"lvremove", "-f", lv.vg.name+"/"+lv.name,
	), nil)
}

// lvnameRegexp is the regexp validating a correct lv name
var lvnameRegexp = regexp.MustCompile("^[A-Za-z0-9_+.][A-Za-z0-9_+.-]*$")

// ValidateLogicalVolumeName validates a volume group name. A valid volume
// group name can consist of a limited range of characters only. The allowed
// characters are [A-Za-z0-9_+.-].
func ValidateLogicalVolumeName(name string) error {
	if !lvnameRegexp.MatchString(name) {
		return ErrInvalidLVName
	}
	return nil
}
