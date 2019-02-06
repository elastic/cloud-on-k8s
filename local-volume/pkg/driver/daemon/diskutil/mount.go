// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package diskutil

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// BindMount bind mounts the source directory to the target directory on the host filesystem
func BindMount(newCmd cmdutil.ExecutableFactory, source string, target string) error {
	cmd := newCmd("mount", "--bind", source, target)
	return cmdutil.RunCmd(cmd)
}

// MountDevice mounts the device at the given path to the given mount path
func MountDevice(newCmd cmdutil.ExecutableFactory, devicePath string, mountPath string) error {
	cmd := newCmd("mount", devicePath, mountPath)
	return cmdutil.RunCmd(cmd)
}

// Unmount unmounts the filesystem at the given mountPath
func Unmount(newCmd cmdutil.ExecutableFactory, mountPath string) error {
	cmd := newCmd("umount", mountPath)
	return cmdutil.RunCmd(cmd)
}
