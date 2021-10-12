// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package math

// RoundUp rounds a value up to the nearest multiple of an other one.
func RoundUp(numToRound, multiple int64) int64 {
	if multiple == 0 {
		return numToRound
	}
	r := numToRound % multiple
	if r == 0 {
		return numToRound
	}
	return numToRound + multiple - r
}
