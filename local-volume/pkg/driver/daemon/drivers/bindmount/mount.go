// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"fmt"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/pathutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Mount mounts a directory to be used as volume by a pod
// The requested storage size is ignored here.
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	// sourceDir is where the directory will be created in the volumes dir
	sourceDir := pathutil.BuildSourceDir(d.mountPath, params.TargetDir)
	if err := diskutil.EnsureDirExists(sourceDir); err != nil {
		return flex.Failure("cannot ensure source directory: " + err.Error())
	}

	// targetDir is going to be a bind mount of sourceDir in the pod directory
	targetDir := params.TargetDir
	if err := diskutil.EnsureDirExists(targetDir); err != nil {
		return flex.Failure("cannot ensure target directory: " + err.Error())
	}

	// create bind mount from source to target dir
	if err := diskutil.BindMount(d.factoryFunc, sourceDir, targetDir); err != nil {
		return flex.Failure(fmt.Sprintf("cannot bind mount %s to %s: %s", sourceDir, targetDir, err.Error()))
	}

	log.Infof("Mounted %s (in-container path) to %s", sourceDir, targetDir)

	return flex.Success("successfully created the volume")
}
