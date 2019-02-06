// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"io/ioutil"
	"os"
	"path"
)

// ListVolumes lists the volumes path to find the existing PVs
func (d *Driver) ListVolumes() ([]string, error) {
	fileinfos, err := ioutil.ReadDir(d.mountPath)
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
	return os.RemoveAll(path.Join(d.mountPath, volumeName))
}
