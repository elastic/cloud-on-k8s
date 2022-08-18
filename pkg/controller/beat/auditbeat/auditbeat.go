// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package auditbeat

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	beatcommon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
)

const (
	Type beatcommon.Type = "auditbeat"
)

type Driver struct {
	beatcommon.DriverParams
	beatcommon.Driver
}

func NewDriver(params beatcommon.DriverParams) beatcommon.Driver {
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() (*reconciler.Results, *beatv1beta1.BeatStatus) {
	managedConfig, err := beatcommon.BuildKibanaConfig(d.Context, d.Client, beatv1beta1.BeatKibanaAssociation{Beat: &d.Beat})
	if err != nil {
		return reconciler.NewResult(d.DriverParams.Context).WithError(err), d.Status
	}

	return beatcommon.Reconcile(d.DriverParams, managedConfig, container.AuditbeatImage)
}
