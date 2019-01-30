// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package diskutil

import (
	"os"
)

// EnsureDirExists checks if the given directory exists,
// or creates it if it doesn't exist
func EnsureDirExists(path string) error {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return os.Mkdir(path, 0755)
	}
	return err
}
