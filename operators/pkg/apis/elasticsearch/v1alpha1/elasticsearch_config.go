// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package v1alpha1

import "strconv"

const (
	NodeData   = "node.data"
	NodeIngest = "node.ingest"
	NodeMaster = "node.master"
	NodeML     = "node.ml"
)

type Config map[string]string

func (c Config) is(key string) bool {
	v, ok := c[key]
	if !ok {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func (c Config) IsMaster() bool {
	return c.is(NodeMaster)
}

func (c Config) IsData() bool {
	return c.is(NodeData)
}

func (c Config) IsIngest() bool {
	return c.is(NodeIngest)
}

func (c Config) IsML() bool {
	return c.is(NodeML)
}

func (c Config) EqualRoles(c2 Config) bool {
	return c.IsMaster() == c2.IsMaster() &&
		c.IsData() == c2.IsData() &&
		c.IsIngest() == c2.IsIngest() &&
		c.IsML() == c2.IsML()
}
