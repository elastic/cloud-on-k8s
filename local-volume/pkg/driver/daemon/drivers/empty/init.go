// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package empty

import (
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/flex"
)

// Init returns a successful response when the driver is ready
func (d *Driver) Init() flex.Response {
	return flex.Response{
		Status:  flex.StatusSuccess,
		Message: "driver is available",
		Capabilities: flex.Capabilities{
			Attach: false, // only implement mount and unmount
		},
	}
}
