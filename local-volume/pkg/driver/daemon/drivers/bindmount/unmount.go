package bindmount

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/diskutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	log "github.com/sirupsen/logrus"
)

// Unmount unmounts the persistent volume
func (d *Driver) Unmount(params protocol.UnmountRequest) flex.Response {
	if err := diskutil.Unmount(params.TargetDir); err != nil {
		return flex.Failure(fmt.Sprintf("Cannot unmount volume %s: %s", params.TargetDir, err.Error()))
	}

	// TODO: rmdir

	log.Infof("Unmounted %s", params.TargetDir)

	return flex.Success("successfully removed the volume")
}
