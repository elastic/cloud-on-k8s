// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package otherbeat

import (
	commonbeat "github.com/elastic/cloud-on-k8s/pkg/controller/common/beat"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

const (
	Type commonbeat.Type = "otherbeat"
)

type Driver struct {
	commonbeat.DriverParams
	commonbeat.Driver
}

func NewDriver(params commonbeat.DriverParams) commonbeat.Driver {
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() commonbeat.DriverResults {
	if d.DaemonSet == nil && d.Deployment == nil {
		d.Deployment = &commonbeat.DeploymentSpec{
			Replicas: pointer.Int32(1),
		}
	}

	return commonbeat.Reconcile(
		d.DriverParams,
		nil,
		"",
		nil)
}
