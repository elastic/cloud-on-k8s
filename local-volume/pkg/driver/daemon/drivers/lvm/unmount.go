package lvm

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Unmount unmounts the PV from the pod dir
func (d *Driver) Unmount(params protocol.UnmountRequest) flex.Response {
	// unmount from the pod dir
	if err := diskutil.Unmount(params.TargetDir); err != nil {
		return flex.Failure(fmt.Sprintf("Cannot unmount volume %s: %s", params.TargetDir, err.Error()))
	}
	log.Infof("Unmounted %s", params.TargetDir)

	// keep the logical volume around for reuse, unmounted

	return flex.Success("successfully unmounted the volume")
}
