// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pipelines

import (
	"fmt"
	"reflect"

	"github.com/elastic/go-ucfg"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"gopkg.in/yaml.v3"
)

// Config contains configuration for Logstash pipeline ("pipelines.yml"),
// `.` in between the key, pipeline.id, is treated as string
// pipelines.yml is expected an array of pipeline definition.
type Config ucfg.Config

// Options are config options for the YAML file.
var Options = []ucfg.Option{ucfg.AppendValues}

// EmptyConfig creates a new empty config.
func EmptyConfig() *Config {
	return fromConfig(ucfg.New())
}

// FromSpec creates a new pipeline from spec.
func FromSpec(cfg interface{}) (*Config, error) {
	config, err := ucfg.NewFrom(cfg, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustFromSpec creates a new pipeline and panics on errors.
// Use for testing only.
func MustFromSpec(cfg interface{}) *Config {
	config, err := FromSpec(cfg)
	if err != nil {
		panic(err)
	}
	return config
}

// Parse parses the given pipeline content into a PipelinesConfig.
// Expects content to be in YAML format.
func Parse(yml []byte) (*Config, error) {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustParse parses the given pipeline content into a Pipelines.
// Expects content to be in YAML format. Panics on error.
// Use for testing only.
func MustParse(yml []byte) *Config {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// Render returns the content of the configuration file,
// with fields sorted alphabetically.
func (c *Config) Render() ([]byte, error) {
	if c == nil {
		return []byte{}, nil
	}
	var out []interface{}
	if err := c.asUCfg().Unpack(&out); err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(out)
}

func (c *Config) asUCfg() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func fromConfig(in *ucfg.Config) *Config {
	return (*Config)(in)
}

// Diff returns true if the key/value or the sequence of two PipelinesConfig are different.
// Use for testing only.
func (c *Config) Diff(c2 *Config) (bool, error) {
	if c == c2 {
		return false, nil
	}
	if c == nil && c2 != nil {
		return true, fmt.Errorf("empty lhs config %s", c2.asUCfg().FlattenedKeys(Options...))
	}
	if c != nil && c2 == nil {
		return true, fmt.Errorf("empty rhs config %s", c.asUCfg().FlattenedKeys(Options...))
	}

	var s []map[string]interface{}
	var s2 []map[string]interface{}
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

// diffSlice returns true if the key/value or the sequence of two PipelinesConfig are different.
func diffSlice(s1, s2 []map[string]interface{}) (bool, error) {
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
