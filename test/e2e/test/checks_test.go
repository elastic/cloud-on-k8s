// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"strings"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
)

func fieldsV1(raw string) *v1.FieldsV1 {
	return v1.NewFieldsV1(raw)
}

func allowedSet(paths ...fieldpath.Path) *fieldpath.Set {
	s := &fieldpath.Set{}
	for _, p := range paths {
		s.Insert(p)
	}
	return s
}

func Test_checkManagedFieldsEntry(t *testing.T) {
	const manager = "elastic-operator"

	tests := []struct {
		name            string
		entry           v1.ManagedFieldsEntry
		manager         string
		allowedPaths    *fieldpath.Set
		wantErr         bool
		wantErrContains []string // each string must appear in the error message
	}{
		{
			name: "skip entry with non-empty subresource",
			entry: v1.ManagedFieldsEntry{
				Manager:     manager,
				Subresource: "status",
				FieldsV1:    fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
			},
			manager: manager,
			wantErr: false,
		},
		{
			name: "skip entry with nil FieldsV1",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: nil,
			},
			manager: manager,
			wantErr: false,
		},
		{
			name: "skip entry from a different manager",
			entry: v1.ManagedFieldsEntry{
				Manager:  "kubectl-client-side-apply",
				FieldsV1: fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
			},
			manager: manager,
			wantErr: false,
		},
		{
			name: "error when manager owns a spec path and allowedPaths is nil",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
			},
			manager:      manager,
			allowedPaths: nil,
			wantErr:      true,
		},
		{
			name: "no error when manager owns only non-spec paths",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:metadata":{"f:annotations":{"f:some-key":{}}}}`),
			},
			manager:      manager,
			allowedPaths: nil,
			wantErr:      false,
		},
		{
			name: "no error when all owned spec paths are in allowedPaths",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:nodeSets":{"k:{\"name\":\"data\"}":{"f:count":{}}}}}`),
			},
			manager: manager,
			allowedPaths: allowedSet(
				fieldpath.MakePathOrDie("spec", "nodeSets", fieldpath.KeyElementByFields("name", "data"), "count"),
			),
			wantErr: false,
		},
		{
			name: "error when at least one spec path is not in allowedPaths",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:nodeSets":{"k:{\"name\":\"data\"}":{"f:count":{},"f:version":{}}}}}`),
			},
			manager: manager,
			allowedPaths: allowedSet(
				fieldpath.MakePathOrDie("spec", "nodeSets", fieldpath.KeyElementByFields("name", "data"), "count"),
			),
			wantErr: true,
		},
		{
			name: "no error when allowedPaths contains a superset of owned spec paths",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:nodeSets":{"k:{\"name\":\"data\"}":{"f:count":{}}}}}`),
			},
			manager: manager,
			allowedPaths: allowedSet(
				fieldpath.MakePathOrDie("spec", "nodeSets", fieldpath.KeyElementByFields("name", "data"), "count"),
				fieldpath.MakePathOrDie("spec", "nodeSets", fieldpath.KeyElementByFields("name", "ml"), "count"),
			),
			wantErr: false,
		},
		{
			name: "error on invalid FieldsV1 JSON",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`not-valid-json`),
			},
			manager: manager,
			wantErr: true,
		},
		{
			name: "error when spec path is owned but not in allowedPaths",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:replicas":{}}}`),
			},
			manager:      manager,
			allowedPaths: &fieldpath.Set{},
			wantErr:      true,
		},
		{
			// This test specifically guards the errors.Join accumulation: each disallowed path
			// must appear in the returned error, not just the last one visited.
			name: "all disallowed spec paths are reported, not only the last one",
			entry: v1.ManagedFieldsEntry{
				Manager:  manager,
				FieldsV1: fieldsV1(`{"f:spec":{"f:foo":{},"f:bar":{}}}`),
			},
			manager:         manager,
			allowedPaths:    nil,
			wantErr:         true,
			wantErrContains: []string{"foo", "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkManagedFieldsEntry(tt.entry, tt.manager, tt.allowedPaths)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkManagedFieldsEntry() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				msg := err.Error()
				for _, want := range tt.wantErrContains {
					if !strings.Contains(msg, want) {
						t.Errorf("expected error to contain %q, got: %s", want, msg)
					}
				}
			}
		})
	}
}
