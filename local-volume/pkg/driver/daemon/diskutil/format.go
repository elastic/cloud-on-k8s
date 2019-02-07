// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package diskutil

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// FormatDevice formats the device at the given path with the given filesystem type
func FormatDevice(newCmd cmdutil.ExecutableFactory, devicePath, fstype string) error {
	cmd := newCmd("mkfs", "-t", fstype, devicePath)
	if err := cmdutil.RunCmd(cmd); err != nil {
		return err
	}
	return nil
}
