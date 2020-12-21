// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
)

// replaceNilNodeRoles works around a rendering issue with 0-sized string arrays/slices
// that have gone through ucfg and have been reduced to nil.
// Elasticsearch's new node.roles syntax relies on empty string arrays in YAML to express
// a coordinating only node. See https://github.com/elastic/cloud-on-k8s/issues/3718
var replaceNilNodeRoles = common.Replacement{
	Path: []string{
		"node", "roles",
	},
	Expected:    nil,
	Replacement: make([]string, 0),
}

// CanonicalConfig contains configuration for Elasticsearch ("elasticsearch.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig struct {
	*common.CanonicalConfig
}

func NewCanonicalConfig() CanonicalConfig {
	return CanonicalConfig{common.NewCanonicalConfig()}
}

// Unpack returns a typed subset of Elasticsearch settings.
func (c CanonicalConfig) Unpack(ver version.Version) (esv1.ElasticsearchSettings, error) {
	cfg := esv1.DefaultCfg(ver)
	err := c.CanonicalConfig.Unpack(&cfg)

	return cfg, err
}
