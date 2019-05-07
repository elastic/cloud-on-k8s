// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package empty

import (
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/flex"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/protocol"
)

// Mount mounts a directory to be used as volume by a pod
// The requested storage size is ignored here.
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	return d.MountRes
}
