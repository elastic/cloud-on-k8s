// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"gopkg.in/yaml.v3"
)

// PipelinesConfig contains configuration for Logstash pipeline ("pipelines.yml"),
// `.` in between the key, pipeline.id, is treated as string
// pipelines.yml is expected an array of pipeline definition
type PipelinesConfig ucfg.Config

// Options are config options for the YAML file.
var Options = []ucfg.Option{ucfg.AppendValues}

// NewPipelinesConfig creates a new empty config.
func NewPipelinesConfig() *PipelinesConfig {
	return fromConfig(ucfg.New())
}

// NewPipelinesConfigFrom creates a new pipeline from spec.
func NewPipelinesConfigFrom(cfg interface{}) (*PipelinesConfig, error) {
	config, err := ucfg.NewFrom(cfg, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustPipelinesConfig creates a new pipeline and panics on errors.
// Use for testing only.
func MustPipelinesConfig(cfg interface{}) *PipelinesConfig {
	config, err := NewPipelinesConfigFrom(cfg)
	if err != nil {
		panic(err)
	}
	return config
}

// ParsePipelinesConfig parses the given pipeline content into a PipelinesConfig.
// Expects content to be in YAML format.
func ParsePipelinesConfig(yml []byte) (*PipelinesConfig, error) {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustParsePipelineConfig parses the given pipeline content into a Pipelines.
// Expects content to be in YAML format. Panics on error.
func MustParsePipelineConfig(yml []byte) *PipelinesConfig {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// Render returns the content of the configuration file,
// with fields sorted alphabetically
func (c *PipelinesConfig) Render() ([]byte, error) {
	if c == nil {
		return []byte{}, nil
	}
	var out []interface{}
	if err := c.asUCfg().Unpack(&out); err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(out)
}

func (c *PipelinesConfig) asUCfg() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func fromConfig(in *ucfg.Config) *PipelinesConfig {
	return (*PipelinesConfig)(in)
}
