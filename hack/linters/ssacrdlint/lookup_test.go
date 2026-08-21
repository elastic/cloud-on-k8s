// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package ssacrdlint

import (
	"go/types"
	"strings"
	"testing"
)

func TestLookupType(t *testing.T) {
	const (
		targetPath = "example.com/target"
		typeName   = "Foo"
	)

	// A package at targetPath with typeName in scope.
	pkgWithType := types.NewPackage(targetPath, "target")
	pkgWithType.Scope().Insert(types.NewTypeName(0, pkgWithType, typeName, types.Typ[types.Int]))

	// A package at targetPath with an empty scope — the case under test.
	pkgWithoutType := types.NewPackage(targetPath, "target")

	makeRoot := func(imports ...*types.Package) *types.Package {
		p := types.NewPackage("example.com/root", "root")
		p.SetImports(imports)
		return p
	}

	tests := []struct {
		name     string
		root     *types.Package
		wantType bool
		wantErr  string
	}{
		{
			name:     "package not in import graph",
			root:     makeRoot(),
			wantType: false,
			wantErr:  "",
		},
		{
			name:     "package found with expected type",
			root:     makeRoot(pkgWithType),
			wantType: true,
			wantErr:  "",
		},
		{
			name:     "package found but type missing",
			root:     makeRoot(pkgWithoutType),
			wantType: false,
			wantErr:  targetPath + "." + typeName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lookupType(tt.root, targetPath, typeName)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				if got != nil {
					t.Fatalf("expected nil type on error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantType && got == nil {
				t.Fatal("expected non-nil type, got nil")
			}
			if !tt.wantType && got != nil {
				t.Fatalf("expected nil type, got %v", got)
			}
		})
	}
}
