// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package drivers

import (
	"fmt"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/bindmount"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/drivers/lvm"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

// DriverKind represents a driver implementation name
type DriverKind string

// Driver interface to be implemented by drivers
type Driver interface {
	Info() string
	Init() flex.Response
	Mount(params protocol.MountRequest) flex.Response
	Unmount(params protocol.UnmountRequest) flex.Response

	// ListVolumes should return the names of PersistentVolumes that are known locally
	ListVolumes() ([]string, error)
	// PurgeVolume should delete the volume associated with the given PersistentVolume name
	PurgeVolume(volumeName string) error
}

// Options defines parameters for the driver creation
type Options struct {
	// BindMountPath Options: only used when the driverKind is BINDMOUNT.
	BindMount bindmount.Options

	// LVM options: only used when the driverKind is LVM.
	LVM lvm.Options
}

// NewDriver creates a driver corresponding to the given driver kind and options
func NewDriver(driverKind string, opts Options) (Driver, error) {
	switch driverKind {
	case bindmount.DriverKind:
		return bindmount.NewDriver(opts.BindMount), nil
	case lvm.DriverKind:
		return lvm.NewDriver(opts.LVM), nil
	default:
		return nil, fmt.Errorf("unsupported driver kind: %s", driverKind)
	}
}
