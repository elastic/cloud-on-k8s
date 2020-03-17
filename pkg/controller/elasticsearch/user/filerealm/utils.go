// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filerealm

import (
	"bytes"
)

// forEachRow applies f to each newline-separated row in the given data.
func forEachRow(data []byte, f func(row []byte) error) error {
	rows := bytes.Split(data, []byte("\n"))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if err := f(row); err != nil {
			return err
		}
	}
	return nil
}
