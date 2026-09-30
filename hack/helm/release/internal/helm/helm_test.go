// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package helm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v4/pkg/registry"
	"oras.land/oras-go/v2/errdef"
)

func TestShouldNotOverwrite(t *testing.T) {
	tests := []struct {
		name          string
		isNonSnapshot bool
		isProdRelease bool
		force         bool
		want          bool
	}{
		{
			name:          "Non-snapshot, prod release, no force",
			isNonSnapshot: true,
			isProdRelease: true,
			force:         false,
			want:          true,
		},
		{
			name:          "Non-snapshot, prod release, force true",
			isNonSnapshot: true,
			isProdRelease: true,
			force:         true,
			want:          false,
		},
		{
			name:          "Snapshot, prod release, no force",
			isNonSnapshot: false,
			isProdRelease: true,
			force:         false,
			want:          false,
		},
		{
			name:          "Non-snapshot, dev release, no force",
			isNonSnapshot: true,
			isProdRelease: false,
			force:         false,
			want:          false,
		},
		{
			name:          "Snapshot, dev release, no force",
			isNonSnapshot: false,
			isProdRelease: false,
			force:         false,
			want:          false,
		},
		{
			name:          "Non-snapshot, dev release, force true",
			isNonSnapshot: true,
			isProdRelease: false,
			force:         true,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldNotOverwrite(tt.isNonSnapshot, tt.isProdRelease, tt.force)
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
				t.Fatalf("failed making temporary directory: %s", err)
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
			if !cmp.Equal(got, tt.want, cmpopts.IgnoreFields(chart{}, "srcPath")) {
				t.Errorf("readCharts() = diff: %s", cmp.Diff(got, tt.want, cmpopts.IgnoreFields(chart{}, "srcPath")))
			}
		})
	}
}

var errWriteFailed = errors.New("write failed")

type errWriter struct{}

func (e *errWriter) Write(_ []byte) (int, error) { return 0, errWriteFailed }

// mockOCIClient is a test double for ociPusher. Resolve reports the refs in existing as found, and every
// resolved and pushed ref is recorded.
// When pushResult is nil, Push returns errPushCalled so tests can confirm a push was attempted
// without needing a real registry. When pushResult is non-nil, Push returns it successfully.
var errPushCalled = errors.New("push called")

type mockOCIClient struct {
	existing     []string
	resolveErr   error
	pushResult   *registry.PushResult
	resolvedRefs []string
	pushedRefs   []string
}

func (m *mockOCIClient) Resolve(ref string) (ocispec.Descriptor, error) {
	m.resolvedRefs = append(m.resolvedRefs, ref)
	if m.resolveErr != nil {
		return ocispec.Descriptor{}, m.resolveErr
	}
	if slices.Contains(m.existing, ref) {
		return ocispec.Descriptor{}, nil
	}
	return ocispec.Descriptor{}, fmt.Errorf("%s: %w", ref, errdef.ErrNotFound)
}

func (m *mockOCIClient) Push(_ []byte, ref string, _ ...registry.PushOption) (*registry.PushResult, error) {
	m.pushedRefs = append(m.pushedRefs, ref)
	if m.pushResult != nil {
		return m.pushResult, nil
	}
	return nil, errPushCalled
}

func TestPushChartToOCI(t *testing.T) {
	dir := t.TempDir()
	newChart := func(name, version string) packagedChart {
		path := filepath.Join(dir, fmt.Sprintf("%s-%s.tgz", name, version))
		if err := os.WriteFile(path, []byte("dummy"), 0600); err != nil {
			t.Fatalf("creating dummy chart archive: %s", err)
		}
		return packagedChart{chart: chart{Name: name, Version: version}, packagePath: path}
	}
	operator := newChart("eck-operator", "1.0.0")
	crds := newChart("eck-operator-crds", "1.0.0")
	snapshot := newChart("eck-operator", "1.0.0-SNAPSHOT")

	const (
		prodRegistry   = "registry.example.invalid/eck"
		devRegistry    = "registry.example.invalid/eck-snapshots"
		operatorRef    = prodRegistry + "/eck-operator-chart:1.0.0"
		crdsRef        = prodRegistry + "/eck-operator-crds-chart:1.0.0"
		snapshotRef    = prodRegistry + "/eck-operator-chart:1.0.0-SNAPSHOT"
		devOperatorRef = devRegistry + "/eck-operator-chart:1.0.0"
		digestSuffix   = "@sha256:abc123\n"
	)

	var successResult registry.PushResult
	if err := json.Unmarshal([]byte(`{"manifest":{"digest":"sha256:abc123"}}`), &successResult); err != nil {
		t.Fatalf("constructing push result: %s", err)
	}
	errNetwork := errors.New("network failure")

	tests := []struct {
		name             string
		charts           []packagedChart
		dev              bool
		dryRun           bool
		force            bool
		existing         []string
		resolveErr       error
		pushFails        bool // Push returns errPushCalled instead of a successful result
		failDigestWriter bool // inject a writer that always errors to test write-failure propagation
		nilDigestWriter  bool // pass nil as the digest writer to exercise the nil-guard
		wantErr          error
		wantErrContains  string
		wantResolvedRefs []string
		wantPushedRefs   []string
		wantDigestOutput string
	}{
		{
			name:             "prod non-snapshot not yet pushed",
			charts:           []packagedChart{operator, crds},
			wantResolvedRefs: []string{operatorRef, crdsRef},
			wantPushedRefs:   []string{operatorRef, crdsRef},
			wantDigestOutput: operatorRef + digestSuffix + crdsRef + digestSuffix,
		},
		{
			name:             "prod non-snapshot already pushed",
			charts:           []packagedChart{operator},
			existing:         []string{operatorRef},
			wantErrContains:  "chart (" + operatorRef + ") already exists in OCI registry (" + prodRegistry + "); remove it or use --force to overwrite",
			wantResolvedRefs: []string{operatorRef},
		},
		{
			name:             "conflict on a later chart stops after earlier charts are pushed",
			charts:           []packagedChart{operator, crds},
			existing:         []string{crdsRef},
			wantErrContains:  "chart (" + crdsRef + ") already exists in OCI registry",
			wantResolvedRefs: []string{operatorRef, crdsRef},
			wantPushedRefs:   []string{operatorRef},
			wantDigestOutput: operatorRef + digestSuffix,
		},
		{
			name:             "resolve error other than not found aborts",
			charts:           []packagedChart{operator},
			resolveErr:       errNetwork,
			wantErr:          errNetwork,
			wantResolvedRefs: []string{operatorRef},
		},
		{
			name:             "prod snapshot is not checked",
			charts:           []packagedChart{snapshot},
			existing:         []string{snapshotRef},
			wantPushedRefs:   []string{snapshotRef},
			wantDigestOutput: snapshotRef + digestSuffix,
		},
		{
			name:             "dev non-snapshot is not checked",
			charts:           []packagedChart{operator},
			dev:              true,
			existing:         []string{devOperatorRef},
			wantPushedRefs:   []string{devOperatorRef},
			wantDigestOutput: devOperatorRef + digestSuffix,
		},
		{
			name:             "force skips checks",
			charts:           []packagedChart{operator},
			force:            true,
			existing:         []string{operatorRef},
			wantPushedRefs:   []string{operatorRef},
			wantDigestOutput: operatorRef + digestSuffix,
		},
		{
			name:             "dry-run checks but does not push",
			charts:           []packagedChart{operator, snapshot},
			dryRun:           true,
			wantResolvedRefs: []string{operatorRef},
		},
		{
			name:             "dry-run reports an existing chart",
			charts:           []packagedChart{operator},
			dryRun:           true,
			existing:         []string{operatorRef},
			wantErrContains:  "chart (" + operatorRef + ") already exists in OCI registry",
			wantResolvedRefs: []string{operatorRef},
		},
		{
			name:           "push error is propagated",
			charts:         []packagedChart{snapshot},
			pushFails:      true,
			wantErr:        errPushCalled,
			wantPushedRefs: []string{snapshotRef},
		},
		{
			name:             "write error on digest file is propagated",
			charts:           []packagedChart{snapshot},
			failDigestWriter: true,
			wantErr:          errWriteFailed,
			wantPushedRefs:   []string{snapshotRef},
		},
		{
			name:            "nil digest writer skips digest output without error",
			charts:          []packagedChart{snapshot},
			nilDigestWriter: true,
			wantPushedRefs:  []string{snapshotRef},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := ReleaseConfig{
				OCIRegistry:   prodRegistry,
				IsProdRelease: !tt.dev,
				DryRun:        tt.dryRun,
				Force:         tt.force,
			}
			if tt.dev {
				conf.OCIRegistry = devRegistry
			}
			var buf strings.Builder
			var digestsWriter io.Writer = &buf
			switch {
			case tt.failDigestWriter:
				digestsWriter = &errWriter{}
			case tt.nilDigestWriter:
				digestsWriter = nil
			}
			client := &mockOCIClient{existing: tt.existing, resolveErr: tt.resolveErr, pushResult: &successResult}
			if tt.pushFails {
				client.pushResult = nil
			}

			var err error
			for _, c := range tt.charts {
				if err = pushChartToOCI(client, conf, digestsWriter, c); err != nil {
					break
				}
			}
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("pushChartToOCI() error = %v, want errors.Is match for %v", err, tt.wantErr)
				}
			case tt.wantErrContains != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
					t.Errorf("pushChartToOCI() error = %v, want error containing %q", err, tt.wantErrContains)
				}
			case err != nil:
				t.Errorf("pushChartToOCI() unexpected error: %s", err)
			}
			if !cmp.Equal(client.resolvedRefs, tt.wantResolvedRefs, cmpopts.EquateEmpty()) {
				t.Errorf("resolved refs diff: %s", cmp.Diff(tt.wantResolvedRefs, client.resolvedRefs, cmpopts.EquateEmpty()))
			}
			if !cmp.Equal(client.pushedRefs, tt.wantPushedRefs, cmpopts.EquateEmpty()) {
				t.Errorf("pushed refs diff: %s", cmp.Diff(tt.wantPushedRefs, client.pushedRefs, cmpopts.EquateEmpty()))
			}
			if got := buf.String(); got != tt.wantDigestOutput {
				t.Errorf("digest output = %q, want %q", got, tt.wantDigestOutput)
			}
		})
	}
}

func TestCheckChartsVersions(t *testing.T) {
	stable := chart{Name: "eck-operator", Version: "1.0.0"}
	snapshot := chart{Name: "eck-operator", Version: "1.0.0-SNAPSHOT"}
	crdsSnapshot := chart{Name: "eck-operator-crds", Version: "2.0.0-SNAPSHOT"}

	tests := []struct {
		name            string
		isProdRelease   bool
		charts          []chart
		wantErr         bool
		wantErrContains []string
	}{
		{
			name:          "dev release allows snapshots",
			isProdRelease: false,
			charts:        []chart{snapshot},
		},
		{
			name:          "prod release allows stable versions",
			isProdRelease: true,
			charts:        []chart{stable},
		},
		{
			name:            "prod release rejects a single snapshot",
			isProdRelease:   true,
			charts:          []chart{snapshot},
			wantErr:         true,
			wantErrContains: []string{snapshot.Name, snapshot.Version},
		},
		{
			name:            "prod release collects all snapshot errors",
			isProdRelease:   true,
			charts:          []chart{snapshot, crdsSnapshot},
			wantErr:         true,
			wantErrContains: []string{snapshot.Name, crdsSnapshot.Name},
		},
		{
			name:            "prod release with mixed versions reports only snapshots",
			isProdRelease:   true,
			charts:          []chart{stable, snapshot},
			wantErr:         true,
			wantErrContains: []string{snapshot.Version},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkChartsVersions(tt.isProdRelease, tt.charts)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkChartsVersions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			for _, want := range tt.wantErrContains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("checkChartsVersions() error = %v, want to contain %q", err, want)
				}
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
		t.Fatalf("failing making directory: %s", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "Chart.yaml"), fmt.Appendf(nil, chartYamlData, name), 0600); err != nil {
		t.Fatalf("failing writing chart file: %s", err)
	}
}
