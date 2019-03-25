// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// FlatConfig contains configuration for Elasticsearch ("elasticsearch.yml"),
// as a flat key-value configuration.
// It does not support hierarchical configuration format.
// For instance:
// * `path.data: /path/to/data` is supported
// * `path:
//       data: /path/to/data` is not supported
type FlatConfig map[string]string

// ParseConfig parses the given configuration content into a FlatConfig.
// Only supports `flat.key: my value` format with one entry per line.
func ParseConfig(content string) (FlatConfig, error) {
	cfg := FlatConfig{}
	for _, line := range strings.Split(content, "\n") {
		// remove spaces (whitespace, tabs, etc.) before and after the setting
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue // ignore empty lines
		}
		if strings.HasPrefix(trimmed, "#") {
			continue // ignore comments
		}
		// split setting name and value
		keyValue := strings.SplitN(trimmed, ":", 2)
		if len(keyValue) != 2 {
			return FlatConfig{}, fmt.Errorf("invalid setting: %s", line)
		}
		// trim key and value
		key := strings.Trim(keyValue[0], " ")
		value := strings.Trim(keyValue[1], " ")
		cfg[key] = value
	}
	return cfg, nil
}

// MergeWith returns a new flat config with the content of c and c2.
// In case of conflict, c2 is taking precedence.
func (c FlatConfig) MergeWith(c2 FlatConfig) FlatConfig {
	newConfig := make(map[string]string, len(c))
	for k, v := range c {
		newConfig[k] = v
	}
	for k, v := range c2 {
		newConfig[k] = v
	}
	return newConfig
}

// Render returns the content of the `elasticsearch.yml` file,
// with fields sorted alphabetically
func (c FlatConfig) Render() []byte {
	var b bytes.Buffer
	b.WriteString("# --- auto-generated ---\n")
	for _, item := range c.Sorted() {
		b.WriteString(item.Key)
		b.WriteString(": ")
		b.WriteString(item.Value)
		b.WriteString("\n")
	}
	b.WriteString("# --- end auto-generated ---\n")
	return b.Bytes()
}

// KeyValue stores a key and a value
type KeyValue struct {
	Key   string
	Value string
}

// Sorted returns a list of KeyValue for this config,
// sorted alphabetically.
func (c FlatConfig) Sorted() []KeyValue {
	// sort keys
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sorted := make([]KeyValue, len(keys))
	for i, k := range keys {
		sorted[i] = KeyValue{Key: k, Value: c[k]}
	}
	return sorted
}
