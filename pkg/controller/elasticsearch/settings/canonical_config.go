// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
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
func (c CanonicalConfig) Unpack() (v1alpha1.ElasticsearchSettings, error) {
	cfg := v1alpha1.DefaultCfg
	err := c.CanonicalConfig.Unpack(&cfg)
	return cfg, err
}
