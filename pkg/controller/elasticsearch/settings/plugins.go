// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"sigs.k8s.io/yaml"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

// PluginsConfigFileName is the file Elasticsearch reads on startup to install, upgrade and remove plugins.
const PluginsConfigFileName = "elasticsearch-plugins.yml"

// pluginsConfig mirrors the schema of elasticsearch-plugins.yml. Elasticsearch rejects unknown keys.
type pluginsConfig struct {
	Plugins []pluginConfig `json:"plugins"`
	Proxy   string         `json:"proxy,omitempty"`
}

type pluginConfig struct {
	ID       string `json:"id"`
	Location string `json:"location,omitempty"`
}

// RenderPluginsConfig renders the elasticsearch-plugins.yml content for the given Elasticsearch resource.
// It returns nil if no plugins are declared.
func RenderPluginsConfig(es esv1.Elasticsearch) ([]byte, error) {
	if !es.HasPlugins() {
		return nil, nil
	}
	cfg := pluginsConfig{
		Plugins: make([]pluginConfig, 0, len(es.Spec.Plugins.Items)),
		Proxy:   es.Spec.Plugins.Proxy,
	}
	for _, p := range es.Spec.Plugins.Items {
		cfg.Plugins = append(cfg.Plugins, pluginConfig{ID: p.ID, Location: p.Location})
	}
	return yaml.Marshal(cfg)
}
