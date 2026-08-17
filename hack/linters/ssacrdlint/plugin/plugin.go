// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Package plugin registers ssacrdlint as a golangci-lint module plugin.
// The custom binary is built with:
//
//	golangci-lint custom
//
// using the .custom-gcl.yml configuration at the repository root.
package plugin

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/elastic/cloud-on-k8s/hack/linters/ssacrdlint"
)

func init() {
	register.Plugin("ssacrdlint", New)
}

type linterPlugin struct{}

func New(_ any) (register.LinterPlugin, error) {
	return &linterPlugin{}, nil
}

func (p *linterPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{ssacrdlint.Analyzer}, nil
}

func (p *linterPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
