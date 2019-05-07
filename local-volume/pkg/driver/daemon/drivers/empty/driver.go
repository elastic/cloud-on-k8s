// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package empty

import (
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/flex"
)

// DriverKind represents the empty driver
const DriverKind = "EMPTY"

// Driver handles empty mounts
type Driver struct {
	MountRes, UnmountRes flex.Response
}

func (d *Driver) Info() string {
	return "Empty driver"
}

func (d *Driver) ListVolumes() ([]string, error) {
	panic("implement me")
}

func (d *Driver) PurgeVolume(volumeName string) error {
	panic("implement me")
}

var _ drivers.Driver = &Driver{}
