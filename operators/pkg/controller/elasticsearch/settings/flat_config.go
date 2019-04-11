// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"reflect"

	"github.com/elastic/go-ucfg"
	yaml2 "github.com/elastic/go-ucfg/yaml"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// FlatConfig contains configuration for Elasticsearch ("elasticsearch.yml"),
// as a flat key-value configuration.
// It does not support hierarchical configuration format.
// For instance:
// * `path.data: /path/to/data` is supported
// * `path:
//       data: /path/to/data` is not supported
type FlatConfig ucfg.Config

var options = []ucfg.Option{ucfg.PathSep(".")}

func NewFlatConfig() *FlatConfig {
	return fromConfig(ucfg.New())
}

func NewFlatConfigFrom(cfg v1alpha1.Config) (*FlatConfig, error) {
	config, err := cfg.Canonicalize()
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

func MustFlatConfig(cfg interface{}) *FlatConfig {
	config, err := ucfg.NewFrom(cfg, options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

func MustNewSingleValue(k string, v ...string) *FlatConfig {
	cfg := fromConfig(ucfg.New())
	err := cfg.Set(k, v...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// ParseConfig parses the given configuration content into a FlatConfig.
// Only supports `flat.key: my value` format with one entry per line.
func ParseConfig(content []byte) (*FlatConfig, error) {
	config, err := yaml2.NewConfig(content, options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil

}

func (c *FlatConfig) Set(key string, vals ...string) error {
	switch len(vals) {
	case 0:
		return errors.New("Nothing to set")
	case 1:
		return c.access().SetString(key, -1, vals[0])
	default:
		for i, v := range vals {
			err := c.access().SetString(key, i, v)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// MergeWith returns a new flat config with the content of c and c2.
// In case of conflict, c2 is taking precedence.
func (c *FlatConfig) MergeWith(cfgs ...*FlatConfig) error {
	for _, c2 := range cfgs {
		err := c.access().Merge(c2.access(), options...)
		if err != nil {
			return err
		}
	}
	return nil
}

// Render returns the content of the `elasticsearch.yml` file,
// with fields sorted alphabetically
func (c *FlatConfig) Render() ([]byte, error) {
	var out map[string]interface{}
	err := c.access().Unpack(&out)
	if err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(out)
}

func (c *FlatConfig) access() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func (c *FlatConfig) Diff(config *FlatConfig, ignore []string) []string {
	var diff []string
	if c == nil && config != nil {
		return config.access().FlattenedKeys(options...)
	}
	if c != nil && config == nil {
		return config.access().FlattenedKeys(options...)
	}
	var l, r = ucfg.MustNewFrom(c.access(), options...), ucfg.MustNewFrom(config.access(), options...)
	for _, i := range ignore {
		_, _ = l.Remove(i, -1, options...)
		_, _ = r.Remove(i, -1, options...)
	}
	var lUnpacked map[string]interface{}
	var rUnpacked map[string]interface{}
	_ = l.Unpack(&lUnpacked, options...)
	_ = r.Unpack(&rUnpacked, options...)
	if reflect.DeepEqual(lUnpacked, rUnpacked) {
		return diff
	}
	return []string{"implement me properly"}
}

func fromConfig(in *ucfg.Config) *FlatConfig {
	return (*FlatConfig)(in)
}
