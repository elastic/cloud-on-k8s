// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package settings

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	ucfg "github.com/elastic/go-ucfg"
	udiff "github.com/elastic/go-ucfg/diff"
	uyaml "github.com/elastic/go-ucfg/yaml"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// CanonicalConfig contains configuration for an Elastic resource ("elasticsearch.yml" or "kibana.yml"),
// as a hierarchical key-value configuration.
type CanonicalConfig ucfg.Config

// Options are config options for the YAML file. Currently contains only support for dotted keys.
var Options = []ucfg.Option{ucfg.PathSep("."), ucfg.AppendValues}

// NewCanonicalConfig creates a new empty config.
func NewCanonicalConfig() *CanonicalConfig {
	return fromConfig(ucfg.New())
}

// NewCanonicalConfigFrom creates a new config from the API type after normalizing the data.
func NewCanonicalConfigFrom(data untypedDict) (*CanonicalConfig, error) {
	// not great: round trip through yaml to normalize untyped dict before creating config
	// to avoid  numeric differences in configs due to JSON marshalling/deep copies being restricted to float
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	var normalized untypedDict
	if err := yaml.Unmarshal(bytes, &normalized); err != nil {
		return nil, err
	}
	config, err := ucfg.NewFrom(normalized, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustCanonicalConfig creates a new config and panics on errors.
// Use for testing only.
func MustCanonicalConfig(cfg interface{}) *CanonicalConfig {
	config, err := ucfg.NewFrom(cfg, Options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// MustNewSingleValue creates a new config holding a single string value.
// It is NewSingleValue but panics rather than returning errors, largely used for convenience in tests
func MustNewSingleValue(k string, v string) *CanonicalConfig {
	cfg := NewCanonicalConfig()
	err := cfg.asUCfg().SetString(k, -1, v, Options...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// NewSingleValue creates a new config holding a single string value.
func NewSingleValue(k string, v string) (*CanonicalConfig, error) {
	cfg := fromConfig(ucfg.New())
	err := cfg.asUCfg().SetString(k, -1, v, Options...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return cfg, nil
}

// ParseConfig parses the given configuration content into a CanonicalConfig.
// Expects content to be in YAML format.
func ParseConfig(yml []byte) (*CanonicalConfig, error) {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		return nil, err
	}
	return fromConfig(config), nil
}

// MustParseConfig parses the given configuration content into a CanonicalConfig.
// Expects content to be in YAML format. Panics on error.
func MustParseConfig(yml []byte) *CanonicalConfig {
	config, err := uyaml.NewConfig(yml, Options...)
	if err != nil {
		panic(err)
	}
	return fromConfig(config)
}

// SetStrings sets key to string vals in c.  An error is returned if key is invalid.
func (c *CanonicalConfig) SetStrings(key string, vals ...string) error {
	if c == nil {
		return errors.New("config is nil")
	}
	switch len(vals) {
	case 0:
		return errors.New("Nothing to set")
	default:
		for i, v := range vals {
			err := c.asUCfg().SetString(key, i, v, Options...)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Unpack returns a typed config given a struct pointer.
func (c *CanonicalConfig) Unpack(cfg interface{}) error {
	if reflect.ValueOf(cfg).Kind() != reflect.Ptr {
		panic("Unpack expects a struct pointer as argument")
	}
	return c.asUCfg().Unpack(cfg, Options...)
}

// MergeWith merges the content of c and c2.
// In case of conflict, c2 is taking precedence.
func (c *CanonicalConfig) MergeWith(cfgs ...*CanonicalConfig) error {
	for _, c2 := range cfgs {
		if c2 == nil {
			continue
		}
		err := c.asUCfg().Merge(c2.asUCfg(), Options...)
		if err != nil {
			return err
		}
	}
	return nil
}

// HasKeys returns all keys in c that are also in keys
func (c *CanonicalConfig) HasKeys(keys []string) []string {
	var has []string
	for _, s := range keys {
		hasKey, err := c.asUCfg().Has(s, 0, Options...)
		if err != nil || hasKey {
			has = append(has, s)
		}
	}
	return has
}

// HasChildConfig returns true if c has a child config object below key.
func (c *CanonicalConfig) HasChildConfig(key string) bool {
	if c == nil {
		return false
	}
	// We are expecting two kinds of error here:
	// type mismatch: if key is pointing to a primitive value that means we don't have a child config
	// missing path: if the key does not exist in the config we also do not have a child config
	// There should be no other errors thrown by ucfg thus there is no error return type in this function.
	_, err := c.asUCfg().Child(key, -1, Options...)
	return err == nil
}

// Render returns the content of the configuration file,
// with fields sorted alphabetically
func (c *CanonicalConfig) Render() ([]byte, error) {
	if c == nil {
		return []byte{}, nil
	}
	var out untypedDict
	err := c.asUCfg().Unpack(&out)
	if err != nil {
		return []byte{}, err
	}
	return yaml.Marshal(out)
}

type untypedDict = map[string]interface{}

// Diff returns the flattened keys where c and c2 differ.
func (c *CanonicalConfig) Diff(c2 *CanonicalConfig, ignore []string) []string {
	var diff []string
	if c == c2 {
		return diff
	}
	if c == nil && c2 != nil {
		return c2.asUCfg().FlattenedKeys(Options...)
	}
	if c != nil && c2 == nil {
		return c.asUCfg().FlattenedKeys(Options...)
	}
	keyDiff := udiff.CompareConfigs(c.asUCfg(), c2.asUCfg(), Options...)
	diff = append(diff, keyDiff[udiff.Add]...)
	diff = append(diff, keyDiff[udiff.Remove]...)
	diff = removeIgnored(diff, ignore)
	if len(diff) > 0 {
		return diff
	}
	// at this point both configs should contain the same keys but may have different values
	var cUntyped untypedDict
	var c2Untyped untypedDict
	err := c.asUCfg().Unpack(&cUntyped, Options...)
	if err != nil {
		return []string{err.Error()}
	}
	err = c2.asUCfg().Unpack(&c2Untyped, Options...)
	if err != nil {
		return []string{err.Error()}
	}

	diff = diffMap(cUntyped, c2Untyped, "")
	return removeIgnored(diff, ignore)
}

func removeIgnored(diff, toIgnore []string) []string {
	var result []string //nolint:prealloc
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
			l, r, err := asUntypedDict(v, v2)
			if err != nil {
				diff = append(diff, newKey)
			}
			diff = append(diff, diffMap(l, r, newKey)...)
		case []interface{}:
			l, r, err := asUntypedSlice(v, v2)
			if err != nil {
				diff = append(diff, newKey)
			}
			diff = append(diff, diffSlice(l, r, newKey)...)
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
			l, r, err := asUntypedDict(v, v2)
			if err != nil {
				diff = append(diff, newKey)
			}
			diff = append(diff, diffMap(l, r, newKey)...)
		case []interface{}:
			l, r, err := asUntypedSlice(v, v2)
			if err != nil {
				diff = append(diff, newKey)
			}
			diff = append(diff, diffSlice(l, r, newKey)...)
		default:
			if v != v2 {
				diff = append(diff, newKey)
			}
		}
	}
	return diff
}

func asUntypedDict(l, r interface{}) (untypedDict, untypedDict, error) {
	lhs, ok := l.(untypedDict)
	rhs, ok2 := r.(untypedDict)
	if !ok || !ok2 {
		return nil, nil, errors.Errorf("map assertion failed for l: %t r: %t", ok, ok2)
	}
	return lhs, rhs, nil
}

func asUntypedSlice(l, r interface{}) ([]interface{}, []interface{}, error) {
	lhs, ok := l.([]interface{})
	rhs, ok2 := r.([]interface{})
	if !ok || !ok2 {
		return nil, nil, errors.Errorf("slice assertion failed for l: %t r: %t", ok, ok2)
	}
	return lhs, rhs, nil
}

func (c *CanonicalConfig) asUCfg() *ucfg.Config {
	return (*ucfg.Config)(c)
}

func fromConfig(in *ucfg.Config) *CanonicalConfig {
	return (*CanonicalConfig)(in)
}
