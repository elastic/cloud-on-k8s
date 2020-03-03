// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

type clientV8 struct {
	clientV7
}

// Equal returns true if c2 can be considered the same as c
func (c *clientV8) Equal(c2 Client) bool {
	other, ok := c2.(*clientV8)
	if !ok {
		return false
	}
	return c.baseClient.equal(&other.baseClient)
}

var _ Client = &clientV8{}
