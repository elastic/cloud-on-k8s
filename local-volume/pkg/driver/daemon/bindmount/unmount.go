package bindmount

import (
	"os/exec"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/model"
	log "github.com/sirupsen/logrus"
)

// Unmount unmounts the persistent volume
func (d *Driver) Unmount(params model.UnmountRequest) model.Response {
	// We unmount target dir on the host from within the container,
	// by entering the host /proc/1/ns/mnt mount namespace
	cmd := exec.Command("nsenter", "--mount=/hostprocns/mnt", "--", "/bin/umount", params.TargetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return model.Response{
			Status:  model.StatusFailure,
			Message: err.Error(),
		}
	}
	log.Infof("Unmounted %s: %s", params.TargetDir, output)

	return model.Response{
		Status:  model.StatusSuccess,
		Message: "successfully removed the volume",
	}
}
