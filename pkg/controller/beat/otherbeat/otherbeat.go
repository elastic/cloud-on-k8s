// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package otherbeat

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

type Driver struct {
	beatcommon.DriverParams
	beatcommon.Driver
}

func NewDriver(params beatcommon.DriverParams) beatcommon.Driver {
	spec := params.Beat.Spec
	// use the default for otherbeat type if not provided
	if spec.DaemonSet == nil && spec.Deployment == nil {
		spec.Deployment = &beatv1beta1.DeploymentSpec{
			Replicas: pointer.Int32(1),
		}
	}
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() (*beatcommon.DriverStatus, *reconciler.Results) {
	return beatcommon.Reconcile(d.DriverParams, nil, "", nil)
}
