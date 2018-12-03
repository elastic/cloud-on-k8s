package diskutil

import (
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// FormatDevice formats the device at the given path with the given filesystem type
func FormatDevice(newCmd cmdutil.FactoryFunc, devicePath, fstype string) error {
	cmd := newCmd("mkfs", "-t", fstype, devicePath)
	if err := cmdutil.RunCmd(cmd); err != nil {
		return err
	}
	return nil
}
