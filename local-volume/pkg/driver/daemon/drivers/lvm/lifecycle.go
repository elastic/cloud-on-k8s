package lvm

import (
	"fmt"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"os/exec"
)

// ListKnownPVs lists the logical volumes from the volume group name to find the PV names
func (d *Driver) ListKnownPVs() ([]string, error) {
	result := lvsOutput{}
	cmd := exec.Command("lvs", "--options=lv_name", d.volumeGroupName, "--reportformat=json", "--nosuffix")

	if err := cmdutil.RunLVMCmd(cmd, &result); err != nil {
		if isVolumeGroupNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	var knownNames []string
	for _, report := range result.Report {
		for _, lv := range report.Lv {
			knownNames = append(knownNames, lv.Name)
		}
	}
	return knownNames, nil
}

// Purge deletes a logical volume
func (d *Driver) Purge(pvName string) error {
	vg, err := LookupVolumeGroup(d.volumeGroupName)
	if err != nil {
		return fmt.Errorf("volume group %s does not seem to exist", d.volumeGroupName)
	}

	lv, err := vg.LookupLogicalVolume(pvName)
	if err != nil {
		if err == ErrLogicalVolumeNotFound {
			// we're deleting, so not found is fine.
			return nil
		}
		return err
	}

	return lv.Remove()
}
