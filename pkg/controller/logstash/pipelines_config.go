// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"fmt"
	"reflect"

	ucfg "github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v3"
)

// PipelinesConfig contains configuration for Logstash pipeline ("pipelines.yml"),
// as array of map of string
type PipelinesConfig ucfg.Config

// Options are config options for the YAML file.
var Options = []ucfg.Option{ucfg.AppendValues}

// NewPipelinesConfig creates a new empty config.
func NewPipelinesConfig() *PipelinesConfig {
	return fromConfig(ucfg.New())
}

// NewPipelinesConfigFrom creates a new pipeline from the API type after normalizing the data.
func NewPipelinesConfigFrom(data []map[string]string) (*PipelinesConfig, error) {
	config, err := ucfg.NewFrom(data, Options...)
	if err != nil {
		return nil, err
	}
	if err := checkTypeArray(config); err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustPipelinesConfig creates a new pipeline and panics on errors.
// Use for testing only.
func MustPipelinesConfig(cfg interface{}) *PipelinesConfig {
	config, err := ucfg.NewFrom(cfg, Options...)
	if err != nil {
		panic(err)
	}
	if err := checkTypeArray(config); err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// ParsePipelinesConfig parses the given pipeline content into a PipelinesConfig.
// Expects content to be in YAML format.
func ParsePipelinesConfig(yml []byte) (*PipelinesConfig, error) {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		return nil, err
	}

	if err := checkTypeArray(config); err != nil {
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
	if err := checkTypeArray(config); err != nil {
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

// Diff returns the flattened keys where c and c2 differ.
// This is used in test only
func (c *PipelinesConfig) Diff(c2 *PipelinesConfig) (bool, error) {
	if c == c2 {
		return false, nil
	}
	if c == nil && c2 != nil {
		return true, fmt.Errorf("empty lhs config %s", c2.asUCfg().FlattenedKeys(Options...))
	}
	if c != nil && c2 == nil {
		return true, fmt.Errorf("empty rhs config %s", c.asUCfg().FlattenedKeys(Options...))
	}

	var s []map[string]string
	var s2 []map[string]string
	err := c.asUCfg().Unpack(&s, Options...)
	if err != nil {
		return true, err
	}
	err = c2.asUCfg().Unpack(&s2, Options...)
	if err != nil {
		return true, err
	}

	return diffSlice(s, s2)
}

func diffSlice(s1, s2 []map[string]string) (bool, error) {
	if len(s1) != len(s2) {
		return true, fmt.Errorf("array size doesn't match %d, %d", len(s1), len(s2))
	}
	var diff []string
	for i, m := range s1 {
		m2 := s2[i]
		if eq := reflect.DeepEqual(m, m2); !eq {
			diff = append(diff, fmt.Sprintf("%s vs %s, ", m, m2))
		}
	}

	if len(diff) > 0 {
		return true, fmt.Errorf("there are %d differences. %s", len(diff), diff)
	}

	return false, nil
}

func (c *PipelinesConfig) asUCfg() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func fromConfig(in *ucfg.Config) *PipelinesConfig {
	return (*PipelinesConfig)(in)
}

// checkTypeArray checks if config is an Array or empty, otherwise throws error
func checkTypeArray(config *ucfg.Config) error {
	if config.IsDict() {
		return errors.New("config is not an array")
	}
	return nil
}
