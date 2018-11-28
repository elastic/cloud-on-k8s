package bindmount

import (
	"io/ioutil"
	"os"
	"path"
)

// ListVolumes lists the volumes path to find the existing PVs
func (d *Driver) ListVolumes() ([]string, error) {
	fileinfos, err := ioutil.ReadDir(ContainerMountPath)
	if err != nil {
		return nil, err
	}

	knownNames := make([]string, len(fileinfos))
	for i, fileinfo := range fileinfos {
		knownNames[i] = fileinfo.Name()
	}

	return knownNames, nil
}

// PurgeVolume recursively deletes the local volume directory
func (d *Driver) PurgeVolume(volumeName string) error {
	return os.RemoveAll(path.Join(ContainerMountPath, volumeName))
}
