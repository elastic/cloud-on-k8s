// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package plugin registers ssacrlint as a golangci-lint module plugin.
// The custom binary is built with:
//
//	golangci-lint custom
//
// using the .custom-gcl.yml configuration at the repository root.
package plugin

import (
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint"
)

func init() {
	register.Plugin("ssacrlint", New)
}

type linterPlugin struct {
	crPathPattern string // empty means "use the analyzer default"
}

// New creates the plugin. settings is the parsed content of the
// linters.settings.custom.ssacrlint.settings block from .golangci.yml.
// The only recognised key is cr-path-pattern (string). Omit it to use the
// default, which matches pkg/apis/ packages under the ECK module root.
func New(settings any) (register.LinterPlugin, error) {
	p := &linterPlugin{}
	if settings == nil {
		return p, nil
	}
	m, ok := settings.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ssacrlint: settings must be a YAML mapping, got %T", settings)
	}
	if v, ok := m["cr-path-pattern"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("ssacrlint: cr-path-pattern must be a string, got %T", v)
		}
		p.crPathPattern = s
	}
	return p, nil
}

func (p *linterPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	a := ssacrlint.NewAnalyzer()
	if p.crPathPattern != "" {
		if err := a.Flags.Set("cr-path-pattern", p.crPathPattern); err != nil {
			return nil, fmt.Errorf("ssacrlint: setting cr-path-pattern: %w", err)
		}
	}
	return []*analysis.Analyzer{a}, nil
}

func (p *linterPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
