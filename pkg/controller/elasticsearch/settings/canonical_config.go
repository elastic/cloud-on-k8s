// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	common "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

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
