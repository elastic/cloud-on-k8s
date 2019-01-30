// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package empty

import (
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

// Unmount unmounts the persistent volume
func (d *Driver) Unmount(params protocol.UnmountRequest) flex.Response {
	return d.UnmountRes
}
