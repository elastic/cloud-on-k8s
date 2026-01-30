// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestShouldNotOverwrite(t *testing.T) {
	tests := []struct {
		name           string
		isNonSnapshot  bool
		isProdHelmRepo bool
		force          bool
		want           bool
	}{
		{
			name:           "Non-snapshot, prod repo, no force",
			isNonSnapshot:  true,
			isProdHelmRepo: true,
			force:          false,
			want:           true,
		},
		{
			name:           "Non-snapshot, prod repo, force true",
			isNonSnapshot:  true,
			isProdHelmRepo: true,
			force:          true,
			want:           false,
		},
		{
			name:           "Snapshot, prod repo, no force",
			isNonSnapshot:  false,
			isProdHelmRepo: true,
			force:          false,
			want:           false,
		},
		{
			name:           "Non-snapshot, dev repo, no force",
			isNonSnapshot:  true,
			isProdHelmRepo: false,
			force:          false,
			want:           false,
		},
		{
			name:           "Snapshot, dev repo, no force",
			isNonSnapshot:  false,
			isProdHelmRepo: false,
			force:          false,
			want:           false,
		},
		{
			name:           "Non-snapshot, dev repo, force true",
			isNonSnapshot:  true,
			isProdHelmRepo: false,
			force:          true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldNotOverwrite(tt.isNonSnapshot, tt.isProdHelmRepo, tt.force)
			if got != tt.want {
				t.Errorf("shouldNotOverwrite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_readCharts(t *testing.T) {
	tests := []struct {
		name          string
		existingPath  string
		chartsToWrite []string
		want          []chart
		wantErr       bool
	}{
		{
			name:          "reads all charts",
			chartsToWrite: []string{"chart1"},
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
			got, err := readCharts(dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("readCharts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(chart{}, "fullPath")) {
				t.Errorf("readCharts() = diff: %s", cmp.Diff(got, tt.want, cmpopts.IgnoreFields(chart{}, "fullPath")))
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
	if err := os.WriteFile(filepath.Join(dir, name, "Chart.yaml"), fmt.Appendf(nil, chartYamlData, name), 0600); err != nil {
		t.Errorf("failing writing chart file: %s", err)
	}
}
