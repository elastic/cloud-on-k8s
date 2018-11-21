package bindmount

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/model"
	log "github.com/sirupsen/logrus"
)

// Mount mounts a directory to be used as volume by a pod
func (d *Driver) Mount(params model.MountRequest) model.Response {
	// sourceDir is where the directory will be created in the volumes dir
	sourceDir := buildSourceDir(params.TargetDir)
	if err := ensureDirExists(sourceDir); err != nil {
		return model.Response{
			Status:  model.StatusFailure,
			Message: "cannot ensure source directory: " + err.Error(),
		}
	}

	// targetDir is going to be a bind mount of sourceDir in the pod directory
	targetDir := params.TargetDir
	if err := ensureDirExists(targetDir); err != nil {
		return model.Response{
			Status:  model.StatusFailure,
			Message: "cannot ensure target directory: " + err.Error(),
		}
	}

	// create bind mount
	err := bindMount(sourceDir, targetDir)
	if err != nil {
		return model.Response{
			Status:  model.StatusFailure,
			Message: fmt.Sprintf("cannot bind mount %s to %s: %s", sourceDir, targetDir, err.Error()),
		}
	}

	log.Infof("Mounted %s to %s", sourceDir, targetDir)

	return model.Response{
		Status:  model.StatusSuccess,
		Message: "successfully created the volume",
	}
}

// bindMount mount the source directory to the target directory on the host filesystem
func bindMount(source string, target string) error {
	// We bind mount source dir to target dir on the host from within the container,
	// by entering the host /proc/1/ns/mnt mount namespace
	cmd := exec.Command("nsenter", "--mount=/hostprocns/mnt", "--", "mount", "--bind", source, target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(string(output[:]))
	}
	return nil
}

// buildSourceDir builds the path to create the volume into,
// eg. /mnt/elastic-local-volumes/<pvc-name>
//
// TODO: unit test
func buildSourceDir(targetDir string) string {
	return path.Join(model.VolumesPath, extractPVCID(targetDir))
}

// extractPVCID returns the last part of the pod volume path given by kubelet,
// corresponding to the PVC ID
//
// eg. from "/var/lib/kubelet/pods/cb528df9-ecab-11e8-be23-080027de035f/volumes/volumes.k8s.elastic.co~elastic-local/pvc-cc6199eb-eca0-11e8-be23-080027de035f"
// we want to return "pvc-cc6199eb-eca0-11e8-be23-080027de035f"
//
// TODO: unit test
func extractPVCID(targetDir string) string {
	return path.Base(targetDir)
}

// ensureDirExists checks if the given directory exists,
// or creates it if it doesn't exist
func ensureDirExists(path string) error {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return os.Mkdir(path, 0755)
	}
	return err
}
