// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package plugin_test

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint"
	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrlint/plugin"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		settings any
		wantErr  bool
	}{
		{name: "nil settings uses defaults", settings: nil},
		{name: "valid cr-path-pattern", settings: map[string]any{"cr-path-pattern": `^github\.com/test/`}},
		{name: "unknown key is ignored", settings: map[string]any{"unknown-key": "value"}},
		{name: "settings is not a map", settings: "not-a-map", wantErr: true},
		{name: "cr-path-pattern wrong type", settings: map[string]any{"cr-path-pattern": 42}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := plugin.New(tt.settings)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if p != nil {
					t.Fatal("expected nil plugin on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p == nil {
				t.Fatal("expected non-nil plugin")
			}
		})
	}
}

func TestBuildAnalyzers(t *testing.T) {
	tests := []struct {
		name        string
		settings    any
		wantPattern string // empty means DefaultCRPathPattern
	}{
		{
			name:        "nil settings returns analyzer with default pattern",
			settings:    nil,
			wantPattern: ssacrlint.DefaultCRPathPattern,
		},
		{
			name:        "custom pattern is forwarded to the analyzer flag",
			settings:    map[string]any{"cr-path-pattern": `^github\.com/test/`},
			wantPattern: `^github\.com/test/`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := plugin.New(tt.settings)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			analyzers, err := p.BuildAnalyzers()
			if err != nil {
				t.Fatalf("BuildAnalyzers: %v", err)
			}
			if len(analyzers) != 1 {
				t.Fatalf("expected 1 analyzer, got %d", len(analyzers))
			}
			a := analyzers[0]
			if a.Name != "ssacrlint" {
				t.Errorf("analyzer name: got %q, want %q", a.Name, "ssacrlint")
			}
			fl := a.Flags.Lookup("cr-path-pattern")
			if fl == nil {
				t.Fatal("cr-path-pattern flag not found on analyzer")
			}
			if got := fl.Value.String(); got != tt.wantPattern {
				t.Errorf("cr-path-pattern: got %q, want %q", got, tt.wantPattern)
			}
		})
	}
}

func TestGetLoadMode(t *testing.T) {
	p, err := plugin.New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := p.GetLoadMode(); got != register.LoadModeTypesInfo {
		t.Errorf("GetLoadMode: got %q, want %q", got, register.LoadModeTypesInfo)
	}
}
