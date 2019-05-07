// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/cmdutil"
)

const (
	// DriverKind represents the bind mount driver
	DriverKind = "BINDMOUNT"

	// DefaultContainerMountPath is the path to access volumes from within the container
	DefaultContainerMountPath = "/mnt/elastic-local-volumes"
)

// Driver handles bind mounts
type Driver struct {
	factoryFunc cmdutil.ExecutableFactory
	mountPath   string
}

// Options for the BindMount driver.
type Options struct {
	Factory   cmdutil.ExecutableFactory
	MountPath string
}

// Info returns some information about the driver
func (d *Driver) Info() string {
	return "Bind mount driver"
}

// NewDriver creates a new bindmount.Driver
func NewDriver(opts Options) *Driver {
	return &Driver{
		factoryFunc: opts.Factory,
		mountPath:   opts.MountPath,
	}
}
