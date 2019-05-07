// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"fmt"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/flex"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Unmount unmounts the persistent volume
func (d *Driver) Unmount(params protocol.UnmountRequest) flex.Response {
	if err := diskutil.Unmount(d.factoryFunc, params.TargetDir); err != nil {
		return flex.Failure(fmt.Sprintf("Cannot unmount volume %s: %s", params.TargetDir, err.Error()))
	}

	// TODO: rmdir

	log.Infof("Unmounted %s", params.TargetDir)

	return flex.Success("successfully removed the volume")
}
