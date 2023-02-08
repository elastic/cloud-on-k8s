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
	tests := []struct {
		name          string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := os.MkdirTemp(os.TempDir(), "readCharts")
			if err != nil {
				t.Errorf(fmt.Sprintf("failed making temporary directory: %s", err))
				return
			}
			defer os.RemoveAll(dir)
			for _, ch := range tt.chartsToWrite {
				mustWriteChart(t, dir, ch)
			}
			got, err := readCharts(dir, tt.excludes)
			if (err != nil) != tt.wantErr {
				t.Errorf("readCharts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readCharts() = diff: %s", cmp.Diff(got, tt.want))
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
