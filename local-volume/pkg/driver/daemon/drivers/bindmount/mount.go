package bindmount

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/pathutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Mount mounts a directory to be used as volume by a pod
func (d *Driver) Mount(params protocol.MountRequest) flex.Response {
	// sourceDir is where the directory will be created in the volumes dir
	sourceDir := pathutil.BuildSourceDir(params.TargetDir)
	if err := diskutil.EnsureDirExists(sourceDir); err != nil {
		return flex.Failure("cannot ensure source directory: " + err.Error())
	}

	// targetDir is going to be a bind mount of sourceDir in the pod directory
	targetDir := params.TargetDir
	if err := diskutil.EnsureDirExists(targetDir); err != nil {
		return flex.Failure("cannot ensure target directory: " + err.Error())
	}

	// create bind mount
	if err := diskutil.BindMount(sourceDir, targetDir); err != nil {
		return flex.Failure(fmt.Sprintf("cannot bind mount %s to %s: %s", sourceDir, targetDir, err.Error()))
	}

	log.Infof("Mounted %s to %s", sourceDir, targetDir)

	return flex.Success("successfully created the volume")
}
