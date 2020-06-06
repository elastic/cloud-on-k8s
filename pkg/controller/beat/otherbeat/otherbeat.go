// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package otherbeat

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

type Driver struct {
	common.DriverParams
	common.Driver
}

func NewDriver(params common.DriverParams) common.Driver {
	spec := &params.Beat.Spec
	// use the default for otherbeat type if not provided
	if spec.DaemonSet == nil && spec.Deployment == nil {
		spec.Deployment = &beatv1beta1.DeploymentSpec{
			Replicas: pointer.Int32(1),
		}
	}
	return &Driver{DriverParams: params}
}

func (d *Driver) Reconcile() *reconciler.Results {
	return common.Reconcile("", d.DriverParams, common.Preset{})
}
