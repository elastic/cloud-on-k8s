package diskutil

import (
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"os/exec"
)

// BindMount bind mounts the source directory to the target directory on the host filesystem
func BindMount(source string, target string) error {
	cmd := exec.Command("mount", "--bind", source, target)
	return cmdutil.RunCmd(cmd)
}

// MountDevice mounts the device at the given path to the given mount path
func MountDevice(devicePath string, mountPath string) error {
	cmd := exec.Command("mount", devicePath, mountPath)
	return cmdutil.RunCmd(cmd)
}

// Unmount unmounts the filesystem at the given mountPath
func Unmount(mountPath string) error {
	cmd := exec.Command("umount", mountPath)
	return cmdutil.RunCmd(cmd)
}
