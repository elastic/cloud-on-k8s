// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"sort"
	"strconv"
	"strings"

	"github.com/elastic/go-ucfg"
	udiff "github.com/elastic/go-ucfg/diff"
	uyaml "github.com/elastic/go-ucfg/yaml"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// CanonicalConfig contains configuration for Elasticsearch ("elasticsearch.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig ucfg.Config

var options = estype.CfgOptions

// NewCanonicalConfig creates a new empty config.
func NewCanonicalConfig() *CanonicalConfig {
	return fromConfig(ucfg.New())
}

// NewCanonicalConfigFrom creates a new config from the API type.
func NewCanonicalConfigFrom(cfg estype.Config) (*CanonicalConfig, error) {
	config, err := ucfg.NewFrom(cfg.Data, options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustCanonicalConfig creates a new config and panics on errors.
// Use for testing only.
func MustCanonicalConfig(cfg interface{}) *CanonicalConfig {
	config, err := ucfg.NewFrom(cfg, options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// MustNewSingleValue creates a new config holding a single string value.
// Convenience constructor, will panic in the unlikely event of errors.
func MustNewSingleValue(k string, v ...string) *CanonicalConfig {
	cfg := fromConfig(ucfg.New())
	err := cfg.Set(k, v...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// ParseConfig parses the given configuration content into a CanonicalConfig.
// Expects content to be in YAML format.
func ParseConfig(yml []byte) (*CanonicalConfig, error) {
	config, err := uyaml.NewConfig(yml, options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil

}

// Set sets key to string vals in c.  An error is returned if key is invalid.
func (c *CanonicalConfig) Set(key string, vals ...string) error {
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

// Unpack returns a typed subset of Elasticsearch settings.
func (c *CanonicalConfig) Unpack() (estype.ElasticsearchSettings, error) {
	cfg := estype.DefaultCfg
	return cfg, c.access().Unpack(&cfg, options...)
}

// MergeWith returns a new canonical config with the content of c and c2.
// In case of conflict, c2 is taking precedence.
func (c *CanonicalConfig) MergeWith(cfgs ...*CanonicalConfig) error {
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

// HasPrefixes returns all keys in c that have one of the given prefix keys.
// Keys are expected in dotted form.
func (c *CanonicalConfig) HasPrefixes(keys []string) []string {
	var has []string
	flatKeys := c.access().FlattenedKeys(options...)
	for _, s := range keys {
		for _, k := range flatKeys {
			if strings.HasPrefix(k, s) {
				has = append(has, s)
			}
		}
	}
	return has
}

// Render returns the content of the `elasticsearch.yml` file,
// with fields sorted alphabetically
func (c *CanonicalConfig) Render() ([]byte, error) {
	if c == nil {
		return []byte{}, nil
	}
	var out untypedDict
	err := c.access().Unpack(&out)
	if err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(out)
}

type untypedDict = map[string]interface{}

// Diff returns the flattened keys whre c and c2 differ.
func (c *CanonicalConfig) Diff(c2 *CanonicalConfig, ignore []string) []string {
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
		return removeIgnored(diff, ignore)
	}
	// at this point both configs should contain the same keys but may have different values
	var cUntyped untypedDict
	var c2Untyped untypedDict
	err := c.access().Unpack(&cUntyped, options...)
	if err != nil {
		return []string{err.Error()}
	}
	err = c2.access().Unpack(&c2Untyped, options...)
	if err != nil {
		return []string{err.Error()}
	}

	diff = diffMap(cUntyped, c2Untyped, "")
	return removeIgnored(diff, ignore)
}

func removeIgnored(diff, toIgnore []string) []string {
	var result []string
	for _, d := range diff {
		if canIgnore(d, toIgnore) {
			continue
		}
		result = append(result, d)
	}
	sort.StringSlice(result).Sort()
	return result
}

func canIgnore(diff string, toIgnore []string) bool {
	for _, prefix := range toIgnore {
		if strings.HasPrefix(diff, prefix) {
			return true
		}
	}
	return false
}

func diffMap(c1, c2 untypedDict, key string) []string {
	// invariant: keys match
	// invariant: json-style map i.e no structs no pointers
	var diff []string
	for k, v := range c1 {
		newKey := k
		if len(key) != 0 {
			newKey = key + "." + k
		}
		v2 := c2[k]
		switch v.(type) {
		case untypedDict:
			diff = append(diff, diffMap(v.(untypedDict), v2.(untypedDict), newKey)...)
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
	// invariant: keys match
	// invariant: s,s2 are json-style arrays/slices i.e no structs no pointers
	if len(s) != len(s2) {
		return []string{key}
	}
	var diff []string
	for i, v := range s {
		v2 := s2[i]
		newKey := key + "." + strconv.Itoa(i)
		switch v.(type) {
		case untypedDict:
			diff = append(diff, diffMap(v.(untypedDict), v2.(untypedDict), newKey)...)
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

func (c *CanonicalConfig) access() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func fromConfig(in *ucfg.Config) *CanonicalConfig {
	return (*CanonicalConfig)(in)
}
