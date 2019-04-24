// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

// Quorum computes the quorum of a cluster given the number of masters.
func Quorum(nMasters int) int {
	if nMasters == 0 {
		return 0
	}
	return (nMasters / 2) + 1
}
