// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"strconv"

	"github.com/elastic/go-ucfg"
	udiff "github.com/elastic/go-ucfg/diff"
	yaml2 "github.com/elastic/go-ucfg/yaml"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
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
	if c == nil {
		return errors.New("config is nil")
	}
	switch len(vals) {
	case 0:
		return errors.New("Nothing to set")
	case 1:
		return c.access().SetString(key, -1, vals[0], options...)
	default:
		for i, v := range vals {
			err := c.access().SetString(key, i, v, options...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *FlatConfig) Unpack() (v1alpha1.ElasticsearchSettings, error) {
	var cfg v1alpha1.ElasticsearchSettings
	return cfg, c.access().Unpack(&cfg, options...)
}

// MergeWith returns a new flat config with the content of c and c2.
// In case of conflict, c2 is taking precedence.
func (c *FlatConfig) MergeWith(cfgs ...*FlatConfig) error {
	for _, c2 := range cfgs {
		if c2 == nil {
			continue
		}
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
	if c == nil {
		return []byte{}, nil
	}
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

func (c *FlatConfig) Diff(c2 *FlatConfig, ignore []string) []string {
	var diff []string
	if c == c2 {
		return diff
	}
	if c == nil && c2 != nil {
		return c2.access().FlattenedKeys(options...)
	}
	if c != nil && c2 == nil {
		return c.access().FlattenedKeys(options...)
	}
	keyDiff := udiff.CompareConfigs(c.access(), c2.access(), options...)
	diff = append(diff, keyDiff[udiff.Add]...)
	diff = append(diff, keyDiff[udiff.Remove]...)
	if len(diff) > 0 {
		return diff
	}
	// at this point both configs should contain the same keys but may have different values
	var lUnpacked map[string]interface{}
	var rUnpacked map[string]interface{}
	err := c.access().Unpack(&lUnpacked, options...)
	if err != nil {
		return []string{err.Error()}
	}
	err = c2.access().Unpack(&rUnpacked, options...)
	if err != nil {
		return []string{err.Error()}
	}

	diff = diffMap(lUnpacked, rUnpacked, "")
	for _, s := range ignore {
		diff = stringsutil.RemoveStringInSlice(s, diff)
	}
	return diff
}

func diffMap(c1, c2 map[string]interface{}, key string) []string {
	// invariant: keys match
	// invariant: json-style map
	var diff []string
	for k, v := range c1 {
		newKey := k
		if len(key) != 0 {
			newKey = key + "." + k
		}
		v2 := c2[k]
		switch v.(type) {
		case map[string]interface{}:
			diff = append(diff, diffMap(v.(map[string]interface{}), v2.(map[string]interface{}), newKey)...)
		case []interface{}:
			diff = append(diff, diffSlice(v.([]interface{}), v2.([]interface{}), newKey)...)
		default:
			if v != v2 {
				diff = append(diff, newKey)
			}
		}
	}
	return diff
}

func diffSlice(s, s2 []interface{}, key string) []string {
	if len(s) != len(s2) {
		return []string{key}
	}
	var diff []string
	for i, v := range s {
		v2 := s2[i]
		newKey := key + "." + strconv.Itoa(i)
		switch v.(type) {
		case map[string]interface{}:
			diff = append(diff, diffMap(v.(map[string]interface{}), v2.(map[string]interface{}), newKey)...)
		case []interface{}:
			diff = append(diff, diffSlice(v.([]interface{}), v2.([]interface{}), newKey)...)
		default:
			if v != v2 {
				diff = append(diff, newKey)
			}
		}
	}
	return diff
}

func fromConfig(in *ucfg.Config) *FlatConfig {
	return (*FlatConfig)(in)
}
