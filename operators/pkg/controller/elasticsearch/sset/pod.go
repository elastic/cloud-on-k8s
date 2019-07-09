// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package sset

import "fmt"

func PodName(ssetName string, ordinal int) string {
	return fmt.Sprintf("%s-%d", ssetName, ordinal)
}
