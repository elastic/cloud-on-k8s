// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_separateChartsWithDependencies(t *testing.T) {
	tests := []struct {
		name         string
		charts       []chart
		wantNoDeps   charts
		wantWithDeps charts
	}{
		{
			name: "happy path",
			charts: []chart{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
			},
			wantNoDeps: charts{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
			},
			wantWithDeps: charts{
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
			},
		},
		{
			name: "charts with dependencies that are not direct dependencies, are treated as having no dependencies",
			charts: []chart{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
				{Name: "chart-with-indirect-deps", Dependencies: []dependency{
					{Name: "nginx"},
				}},
			},
			wantNoDeps: charts{
				{Name: "eck-elasticsearch"},
				{Name: "eck-kibana"},
				{Name: "eck-agent"},
				{Name: "eck-beat"},
				{Name: "chart-with-indirect-deps", Dependencies: []dependency{
					{Name: "nginx"},
				}},
			},
			wantWithDeps: charts{
				{Name: "eck-stack", Dependencies: []dependency{
					{Name: "eck-elasticsearch"},
					{Name: "eck-kibana"},
					{Name: "eck-agent"},
					{Name: "eck-beat"},
				}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNoDeps, gotWithDeps := separateChartsWithDependencies(tt.charts)
			if !reflect.DeepEqual(gotNoDeps, tt.wantNoDeps) {
				t.Errorf("separateChartsWithDependencies() gotNoDeps = %s", cmp.Diff(gotNoDeps, tt.wantNoDeps))
			}
			if !reflect.DeepEqual(gotWithDeps, tt.wantWithDeps) {
				t.Errorf("separateChartsWithDependencies() gotWithDeps = %s", cmp.Diff(gotWithDeps, tt.wantWithDeps))
			}
		})
	}
}

func Test_readCharts(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.FailNow()
		return
	}
	tests := []struct {
		name          string
		existingPath  string
		chartsToWrite []string
		excludes      []string
		want          []chart
		wantErr       bool
	}{
		{
			name:          "no excludes reads all charts",
			chartsToWrite: []string{"chart1"},
			excludes:      []string{},
			want: []chart{
				{
					Name:         "chart1",
					Version:      "0.1.0",
					Dependencies: []dependency{},
				},
			},
			wantErr: false,
		},
		{
			name:          "excluded charts is ignored",
			chartsToWrite: []string{"chart1", "excludedChart"},
			excludes:      []string{"excludedChart"},
			want: []chart{
				{
					Name:         "chart1",
					Version:      "0.1.0",
					Dependencies: []dependency{},
				},
			},
			wantErr: false,
		},
		{
			name:          "existing charts is correct",
			existingPath:  filepath.Join(cwd, "..", "..", "..", "..", "..", "deploy", "eck-operator"),
			chartsToWrite: nil,
			excludes:      nil,
			want: []chart{
				{
					Name:    "eck-operator",
					Version: "2.8.0-SNAPSHOT",
					Dependencies: []dependency{
						{
							Name:    "eck-operator-crds",
							Version: "2.8.0-SNAPSHOT",
						},
					},
				},
				{
					Name:         "eck-operator-crds",
					Version:      "2.8.0-SNAPSHOT",
					Dependencies: nil,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.existingPath
			if tt.existingPath == "" {
				dir, err := os.MkdirTemp(os.TempDir(), "readCharts")
				if err != nil {
					t.Errorf(fmt.Sprintf("failed making temporary directory: %s", err))
					return
				}
				path = dir
				for _, ch := range tt.chartsToWrite {
					mustWriteChart(t, dir, ch)
				}
				defer os.RemoveAll(dir)
			} else {
				t.Logf("using path: %s", tt.existingPath)
			}
			got, err := readCharts(path, tt.excludes)
			if (err != nil) != tt.wantErr {
				t.Errorf("readCharts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(chart{}, "fullPath")) {
				t.Errorf("readCharts() = diff: %s", cmp.Diff(got, tt.want, cmpopts.IgnoreFields(chart{}, "fullPath")))
			}
			noDeps, withDeps := separateChartsWithDependencies(got)
			t.Logf("got no deps: %+v", noDeps)
			t.Logf("got deps: %+v", withDeps)
			if len(noDeps)+len(withDeps) != len(got) {
				t.Errorf("separateChartsWithDependencies: total deps (%d) + no deps (%d) != charts from readCharts (%d)", len(withDeps), len(noDeps), len(got))
			}
		})
	}
}

var chartYamlData = `
apiVersion: v2
name: %s
description: Fake Helm Chart
type: application
version: 0.1.0
dependencies: []
`

func mustWriteChart(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
		t.Errorf("failing making directory: %s", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "Chart.yaml"), []byte(fmt.Sprintf(chartYamlData, name)), 0600); err != nil {
		t.Errorf("failing writing chart file: %s", err)
	}
}
